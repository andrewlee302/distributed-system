package twopc

// PreparedArgs is the arg for InformPrepared RPC call.
type PreparedArgs struct {
	TxnID      string
	TxnPartIdx int
	ErrCode    int
}

// PreparedReply is the reply for InformPrepared RPC call.
type PreparedReply struct{}

// AbortedArgs is the arg for InformAborted RPC call.
type AbortedArgs struct {
	TxnID      string
	TxnPartIdx int
	ErrCode    int
}

// AbortedReply is the reply for InformAborted RPC call.
type AbortedReply struct{}

// AbortArgs is the arg for Abort RPC call.
type AbortArgs struct {
	TxnPartID string
}

// AbortReply is the reply for Abort RPC call.
type AbortReply struct{}

// CommitArgs is the arg for Commit RPC call.
type CommitArgs struct {
	TxnPartID string
}

// CommitReply is the reply for Commit RPC call.
type CommitReply struct {
	TxnPartID string
}
