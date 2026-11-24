package windowsinstaller

import "testing"

func TestInstallerArgumentsKeepTheLastDirectory(t *testing.T) {
	for _, raw := range []string{`"C:\下载\setup.exe" /S /LANG=2052 /D=C:\日本語  folder`, `"C:\下载\setup.exe" /S /LANG=2052 "/D=C:\日本語  folder"`, `"C:\下载\setup.exe" /S /LANG=2052 /D="C:\日本語  folder"`} {
		got, err := arguments([]string{"/S", "/LANG=2052", "/D=C:\\日本語", "folder"}, raw)
		if err != nil || !got.silent || got.language != 2052 || got.directory != `C:\日本語  folder` {
			t.Errorf("arguments(%q) = %+v, %v", raw, got, err)
		}
	}
	for _, args := range [][]string{{"/D="}, {"/LANG=unknown"}, {"/LANG=0"}, {"/unexpected"}} {
		if _, err := arguments(args, ""); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}
func TestInstallerLanguageDoesNotRenameTheInstallation(t *testing.T) {
	m := Manifest{Name: "Stable folder", Names: []Name{{Language: 0x0409, Text: "English Name"}, {Language: 0x0804, Text: "中文名称"}}}
	words := installerWords(m, 0x0804)
	if words.Install != "安装" || words.Cancel != "取消" || words.Close != "关闭" || displayName(m, 0x0804) != "中文名称" {
		t.Fatalf("Chinese UI selection: %+v", words)
	}
	if m.Name != "Stable folder" {
		t.Fatal("choosing a UI language renamed the installation")
	}
}
