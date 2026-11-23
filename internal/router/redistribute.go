package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
	"github.com/Alekra1/sharded_key_value_store/internal/protocol"
)

var ErrMaintenanceRunning = errors.New("maintenance is already running")

// AddNode adds a running node to the cluster and moves the keys it now owns.
func (rt *Router) AddNode(ctx context.Context, id placement.NodeID, nodeURL string) error {
	nodeURL = strings.TrimSuffix(strings.TrimSpace(nodeURL), "/")
	if id == "" {
		return errors.New("node id is empty")
	}
	if nodeURL == "" {
		return errors.New("node url is empty")
	}
	if !rt.maint.TryLock() {
		return ErrMaintenanceRunning
	}
	defer rt.maint.Unlock()

	oldMembers, next, oldURLs, err := rt.clonePlacement()
	if err != nil {
		return err
	}
	if err := next.Add(id); err != nil {
		return err
	}

	nextURLs := maps.Clone(oldURLs)
	nextURLs[id] = nodeURL
	return rt.redistribute(ctx, oldMembers, next, nextURLs, nextURLs)
}

// RemoveNode moves all keys off id and removes it from the cluster.
func (rt *Router) RemoveNode(ctx context.Context, id placement.NodeID) error {
	if id == "" {
		return errors.New("node id is empty")
	}
	if !rt.maint.TryLock() {
		return ErrMaintenanceRunning
	}
	defer rt.maint.Unlock()

	oldMembers, next, oldURLs, err := rt.clonePlacement()
	if err != nil {
		return err
	}
	if err := next.Remove(id); err != nil {
		return err
	}

	nextURLs := maps.Clone(oldURLs)
	delete(nextURLs, id)
	return rt.redistribute(ctx, oldMembers, next, nextURLs, oldURLs)
}

func (rt *Router) clonePlacement() ([]placement.NodeID, placement.Placement, map[placement.NodeID]string, error) {
	rt.gate.RLock()
	saved, err := rt.placement.Save()
	members := rt.placement.Members()
	urls := maps.Clone(rt.nodes)
	rt.gate.RUnlock()
	if err != nil {
		return nil, nil, nil, err
	}
	clone, err := placement.Load(saved)
	if err != nil {
		return nil, nil, nil, err
	}
	return members, clone, urls, nil
}

// redistribute moves keys from the current placement to next while client
// requests are paused. If a step fails before the switch, the old placement
// and source records stay untouched and the router remains paused.
func (rt *Router) redistribute(
	ctx context.Context,
	oldMembers []placement.NodeID,
	next placement.Placement,
	nextURLs map[placement.NodeID]string,
	allURLs map[placement.NodeID]string,
) error {
	allIDs := slices.Sorted(maps.Keys(allURLs))

	if err := rt.Pause(); err != nil {
		return fmt.Errorf("pause router: %w", err)
	}
	for _, id := range allIDs {
		if err := rt.postNode(ctx, allURLs[id]+"/freeze"); err != nil {
			return fmt.Errorf("freeze node %s: %w", id, err)
		}
	}

	toCopy := make(map[placement.NodeID]map[string]string)
	toDelete := make(map[placement.NodeID][]string)

	for _, src := range oldMembers {
		records, err := rt.getRecords(ctx, allURLs[src])
		if err != nil {
			return fmt.Errorf("read records from %s: %w", src, err)
		}
		for key, value := range records {
			dst := next.Owner(key)
			if dst == src {
				continue
			}
			if toCopy[dst] == nil {
				toCopy[dst] = make(map[string]string)
			}
			toCopy[dst][key] = value
			toDelete[src] = append(toDelete[src], key)
		}
	}

	for _, dst := range slices.Sorted(maps.Keys(toCopy)) {
		if err := rt.putRecords(ctx, allURLs[dst], toCopy[dst]); err != nil {
			return fmt.Errorf("copy records to %s: %w", dst, err)
		}
	}

	for _, dst := range slices.Sorted(maps.Keys(toCopy)) {
		got, err := rt.getRecords(ctx, allURLs[dst])
		if err != nil {
			return fmt.Errorf("verify records on %s: %w", dst, err)
		}
		for key, wantValue := range toCopy[dst] {
			gotValue, ok := got[key]
			if !ok || gotValue != wantValue {
				return fmt.Errorf("verify failed on %s for key %q", dst, key)
			}
		}
	}

	if err := rt.switchPlacement(next, nextURLs); err != nil {
		return fmt.Errorf("save new placement: %w", err)
	}

	for _, src := range slices.Sorted(maps.Keys(toDelete)) {
		if err := rt.deleteRecords(ctx, allURLs[src], toDelete[src]); err != nil {
			return fmt.Errorf("clean up %s: %w", src, err)
		}
	}
	for _, id := range allIDs {
		if err := rt.postNode(ctx, allURLs[id]+"/unfreeze"); err != nil {
			return fmt.Errorf("unfreeze node %s: %w", id, err)
		}
	}
	if err := rt.Unpause(); err != nil {
		return fmt.Errorf("unpause router: %w", err)
	}
	return nil
}

func (rt *Router) switchPlacement(next placement.Placement, nextURLs map[placement.NodeID]string) error {
	rt.gate.Lock()
	defer rt.gate.Unlock()
	if err := saveState(rt.statePath, next, nextURLs, true); err != nil {
		return err
	}
	rt.placement = next
	rt.nodes = nextURLs
	return nil
}

func (rt *Router) postNode(ctx context.Context, target string) error {
	_, err := rt.doNode(ctx, http.MethodPost, target, nil)
	return err
}

func (rt *Router) getRecords(ctx context.Context, nodeURL string) (map[string]string, error) {
	data, err := rt.doNode(ctx, http.MethodGet, nodeURL+"/records", nil)
	if err != nil {
		return nil, err
	}
	var body protocol.Records
	if err := json.Unmarshal(data, &body); err != nil || body.Records == nil {
		return nil, fmt.Errorf("bad records reply: %q", data)
	}
	return body.Records, nil
}

func (rt *Router) putRecords(ctx context.Context, nodeURL string, records map[string]string) error {
	_, err := rt.doNode(ctx, http.MethodPut, nodeURL+"/records", protocol.Records{Records: records})
	return err
}

func (rt *Router) deleteRecords(ctx context.Context, nodeURL string, keys []string) error {
	_, err := rt.doNode(ctx, http.MethodDelete, nodeURL+"/records", protocol.Keys{Keys: keys})
	return err
}

func (rt *Router) doNode(ctx context.Context, method, target string, body any) ([]byte, error) {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := rt.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		var apiErr protocol.Error
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != "" {
			return nil, errors.New(apiErr.Error)
		}
		return nil, fmt.Errorf("node replied %s", resp.Status)
	}
	return data, nil
}
