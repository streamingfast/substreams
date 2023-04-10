package templates

type Project interface {
	Render() (map[string][]byte, error)
}

type ProjectFunc func() (map[string][]byte, error)

func (f ProjectFunc) Render() (map[string][]byte, error) {
	return f()
}
