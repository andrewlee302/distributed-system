package tinykv

type GetArgs struct {
	Key string
}

type DelArgs GetArgs

type PutArgs struct {
	Key   string
	Value string
}

type IncrArgs struct {
	Key   string
	Delta int
}

type Reply struct {
	Flag  bool
	Value string
}
