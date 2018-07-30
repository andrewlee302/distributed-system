package paxoskv

// Errors for PutAppend and Get RPC call.
const (
	OK           = "OK"
	ErrNoKey     = "ErrNoKey"
	ErrPending   = "ErrPending"
	ErrForgotten = "ErrForgotten"
)

// Three kinds of Ops.
const (
	Get    = "Get"
	Put    = "Put"
	Append = "Append"
)

// PutAppendArgs is the args for PutAppend RPC call.
type PutAppendArgs struct {
	// You'll have to add definitions here.
	Key   string
	Value string
	Op    string // "Put" or "Append"
	ID    int64
	// You'll have to add definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
}

// PutAppendReply is the reply for PutAppend RPC call.
type PutAppendReply struct {
	Err string
}

// GetArgs is the args for Get RPC call.
type GetArgs struct {
	Key string
	// You'll have to add definitions here.
	ID int64
}

// GetReply is the reply for Get RPC call.
type GetReply struct {
	Err   string
	Value string
}
