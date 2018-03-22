package twopc

type PreparedArgs struct {
	TxnID      string
	TxnPartIdx int
	ErrCode    int
}

type PreparedReply struct{}

type AbortedArgs struct {
	TxnID      string
	TxnPartIdx int
	ErrCode    int
}

type AbortedReply struct{}

type PrepareArgs struct {
	TxnPartID string
}

type PrepareReply struct {
	State int32
}

type AbortArgs struct {
	TxnPartID string
}

type AbortReply struct{}

type CommitArgs struct {
	TxnPartID string
}

type CommitReply struct {
	TxnPartID string
}
