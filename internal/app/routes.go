package app

import "net/http"

// routeMux is a ServeMux that records each pattern it registers, so a test can walk every route.
type routeMux struct {
	*http.ServeMux
	patterns *[]string
}

func (m routeMux) Handle(pattern string, h http.Handler) {
	*m.patterns = append(*m.patterns, pattern)
	m.ServeMux.Handle(pattern, h)
}

func (m routeMux) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	m.Handle(pattern, http.HandlerFunc(h))
}
