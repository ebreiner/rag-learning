package inspect

type Inspector interface {
	Stats() (string, error)
	Dump() (string, error)
}
