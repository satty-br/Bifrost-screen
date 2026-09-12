package steam

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// InstallDir acha a pasta onde um jogo da Steam está instalado, pelo AppID,
// procurando em todas as bibliotecas (não só a padrão). Usado pelos recursos
// que precisam escrever dentro da pasta do jogo (ex.: os arquivos de Game
// State Integration do CS2 e do Dota 2).
func InstallDir(appID int) (string, error) {
	root, err := defaultLocalEnv().SteamPath()
	if err != nil || root == "" {
		return "", ErrSteamNotFound
	}
	root = filepath.Clean(root)
	if _, err := os.Stat(root); err != nil {
		return "", ErrSteamNotFound
	}

	libs := []string{root}
	if data, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf")); err == nil {
		if v, err := ParseVDF(string(data)); err == nil {
			if lf := v.Get("libraryfolders"); lf != nil {
				for _, child := range lf.Children {
					if p := child.String("path"); p != "" {
						libs = append(libs, filepath.Clean(p))
					}
				}
			}
		}
	}

	for _, lib := range libs {
		manifest := filepath.Join(lib, "steamapps", fmt.Sprintf("appmanifest_%d.acf", appID))
		data, err := os.ReadFile(manifest)
		if err != nil {
			continue
		}
		v, err := ParseVDF(string(data))
		if err != nil {
			continue
		}
		installDir := v.String("AppState", "installdir")
		if installDir == "" {
			continue
		}
		full := filepath.Join(lib, "steamapps", "common", installDir)
		if _, err := os.Stat(full); err == nil {
			return full, nil
		}
	}
	return "", errors.New("jogo não encontrado em nenhuma biblioteca da Steam")
}
