package lcd

// Identificação USB das telas revisão A.
const (
	revAVID    = "1A86"
	revAPID    = "5722"
	revASerial = "USB35INCHIPSV2"
)

// PortInfo descreve uma porta serial encontrada no sistema.
type PortInfo struct {
	Name     string `json:"nome"`
	VIDPID   string `json:"vid_pid"`
	Serial   string `json:"serial"`
	IsScreen bool   `json:"e_a_tela"`
}

// DetectRevA encontra a porta da tela revisão A.
func DetectRevA() (string, error) {
	ports, err := ListPorts()
	if err != nil {
		return "", err
	}
	for _, p := range ports {
		if p.IsScreen {
			return p.Name, nil
		}
	}
	return "", ErrNotFound
}
