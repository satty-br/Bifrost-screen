package steam

import "testing"

const sampleLocalConfig = `"UserLocalConfigStore"
{
	"friends"
	{
		"PersonaName"		"satty"
	}
	"Software"
	{
		"Valve"
		{
			"Steam"
			{
				"apps"
				{
					"730"
					{
						"LastPlayed"		"1789160000"
						"Playtime2wks"		"340"
						"Playtime"		"5423"
						"cloud" { "last_sync_state" "synchronized" }
					}
					"570"
					{
						"Playtime"		"12"
					}
				}
			}
		}
	}
}`

func TestParseVDF(t *testing.T) {
	v, err := ParseVDF(sampleLocalConfig)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String("UserLocalConfigStore", "friends", "PersonaName"); got != "satty" {
		t.Errorf("persona = %q", got)
	}
	app := v.Get("UserLocalConfigStore", "Software", "Valve", "Steam", "apps", "730")
	if app == nil || app.String("Playtime") != "5423" || app.String("Playtime2wks") != "340" {
		t.Errorf("playtime errado: %+v", app)
	}
	// maiúsculas diferentes devem funcionar
	if v.String("userlocalconfigstore", "SOFTWARE", "valve", "steam", "Apps", "570", "playtime") != "12" {
		t.Error("busca sem diferenciar maiúsculas falhou")
	}
}

func TestParseManifest(t *testing.T) {
	acf := "\"AppState\"\n{\n\t\"appid\"\t\t\"730\"\n\t\"name\"\t\t\"Counter-Strike 2\"\n\t\"installdir\"\t\t\"Counter-Strike Global Offensive\"\n}\n"
	v, err := ParseVDF(acf)
	if err != nil {
		t.Fatal(err)
	}
	if v.String("AppState", "name") != "Counter-Strike 2" {
		t.Error("nome do manifesto errado")
	}
}

func TestParseLibraryFolders(t *testing.T) {
	lf := `"libraryfolders"
{
	"0" { "path" "C:\\Program Files (x86)\\Steam" "apps" { "228980" "0" } }
	"1" { "path" "D:\\SteamLibrary" "apps" { "730" "0" } }
}`
	v, err := ParseVDF(lf)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String("libraryfolders", "1", "path"); got != `D:\SteamLibrary` {
		t.Errorf("path = %q", got)
	}
}
