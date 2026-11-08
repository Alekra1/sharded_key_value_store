package placement

type maglevNode struct {
	name NodeID
	next int
	skip int
}

func maglevTable(members []NodeID, m int) []NodeID {
	nodes := make([]maglevNode, len(members))
	for i, name := range members {
		nodes[i] = maglevNode{
			name: name,
			next: int(hash64(string(name), "offset") % uint64(m)),
			skip: int(hash64(string(name), "skip")%uint64(m-1)) + 1,
		}
	}

	table := make([]NodeID, m)
	filled := 0
	for filled < m {
		for i := range nodes {
			n := &nodes[i]
			slot := n.next
			for table[slot] != "" {
				slot = (slot + n.skip) % m
			}
			table[slot] = n.name
			n.next = (slot + n.skip) % m
			filled++
			if filled == m {
				break
			}
		}
	}
	return table
}
