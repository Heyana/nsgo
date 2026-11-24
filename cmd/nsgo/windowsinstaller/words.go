package windowsinstaller

// The runtime has no dependency on the app's catalogs. Its own words follow the installer languages available to the app; names still come from those catalogs.
type words struct{ Install, Uninstall, Browse, Desktop, Open, Running, Done, Working string }

var english = words{"Install", "Uninstall", "Choose folder…", "Create a desktop shortcut", "Open app", "%s is running. Close it and continue?", "Completed.", "Please wait…"}
var translations = map[uint16]words{
	0x04: {"安装", "卸载", "选择文件夹…", "创建桌面快捷方式", "打开应用", "%s 正在运行。是否关闭并继续？", "已完成。", "请稍候…"},
	0x11: {"インストール", "アンインストール", "フォルダーを選択…", "デスクトップにショートカットを作成", "アプリを開く", "%s は実行中です。終了して続行しますか？", "完了しました。", "お待ちください…"},
	0x12: {"설치", "제거", "폴더 선택…", "바탕 화면에 바로 가기 만들기", "앱 열기", "%s이(가) 실행 중입니다. 종료하고 계속하시겠습니까?", "완료되었습니다.", "잠시 기다려 주세요…"},
	0x07: {"Installieren", "Deinstallieren", "Ordner auswählen…", "Desktopverknüpfung erstellen", "App öffnen", "%s wird ausgeführt. Beenden und fortfahren?", "Abgeschlossen.", "Bitte warten…"},
	0x0c: {"Installer", "Désinstaller", "Choisir un dossier…", "Créer un raccourci sur le bureau", "Ouvrir l’application", "%s est en cours d’exécution. Le fermer et continuer ?", "Terminé.", "Veuillez patienter…"},
	0x0a: {"Instalar", "Desinstalar", "Elegir carpeta…", "Crear un acceso directo en el escritorio", "Abrir aplicación", "%s se está ejecutando. ¿Cerrarlo y continuar?", "Completado.", "Espere…"},
	0x10: {"Installa", "Disinstalla", "Scegli cartella…", "Crea un collegamento sul desktop", "Apri app", "%s è in esecuzione. Chiuderlo e continuare?", "Completato.", "Attendere…"},
	0x16: {"Instalar", "Desinstalar", "Escolher pasta…", "Criar um atalho no ambiente de trabalho", "Abrir aplicação", "%s está em execução. Fechá-lo e continuar?", "Concluído.", "Aguarde…"},
	0x19: {"Установить", "Удалить", "Выбрать папку…", "Создать ярлык на рабочем столе", "Открыть приложение", "%s запущено. Закрыть его и продолжить?", "Готово.", "Подождите…"},
}

func catalogWords(m Manifest, language uint16) words {
	supported := false
	for _, name := range m.Names {
		if name.Language == language {
			supported = true
			break
		}
	}
	if !supported {
		for _, name := range m.Names {
			if name.Language&0x3ff == language&0x3ff {
				supported = true
				break
			}
		}
	}
	if !supported {
		return english
	}
	if language == 0x0404 || language == 0x0c04 || language == 0x1404 {
		return words{"安裝", "解除安裝", "選擇資料夾…", "建立桌面捷徑", "開啟應用程式", "%s 正在執行。是否關閉並繼續？", "已完成。", "請稍候…"}
	}
	if language == 0x0416 {
		return words{"Instalar", "Desinstalar", "Escolher pasta…", "Criar um atalho na área de trabalho", "Abrir aplicativo", "%s está em execução. Fechá-lo e continuar?", "Concluído.", "Aguarde…"}
	}
	if words, ok := moreWords(language); ok {
		return words
	}
	if words, ok := translations[language&0x3ff]; ok {
		return words
	}
	return english
}
