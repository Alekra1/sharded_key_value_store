package protocol

// Value is the JSON body of a PUT request and of a successful GET reply.
type Value struct {
	// A pointer so a body without "value" is refused instead of storing "".
	Value *string `json:"value"`
}

// Error is the JSON body of every failed request.
type Error struct {
	Error string `json:"error"`
}
