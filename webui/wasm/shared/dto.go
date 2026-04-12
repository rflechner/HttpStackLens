package shared

type RequestEventDto struct {
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Path    string `json:"path"`
	Version string `json:"version"`
}
