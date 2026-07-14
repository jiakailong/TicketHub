package base

type Area struct {
	ID       int64
	ParentID int64
	Name     string
	Level    int
	Hot      bool
}

type ChannelData struct {
	ID    int64
	Code  string
	Value string
}
