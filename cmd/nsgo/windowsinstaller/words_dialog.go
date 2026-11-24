package windowsinstaller

// Dialog buttons are bundled too: an explicit /LANG choice must not leave Cancel and Close in the operating system's unrelated display language.
type dialogWords struct {
	words
	Cancel, Close string
}

var buttonTranslations = map[uint16][2]string{
	0x01: {"إلغاء", "إغلاق"}, 0x02: {"Отказ", "Затваряне"}, 0x03: {"Cancel·la", "Tanca"}, 0x04: {"取消", "关闭"},
	0x05: {"Zrušit", "Zavřít"}, 0x06: {"Annuller", "Luk"}, 0x07: {"Abbrechen", "Schließen"}, 0x08: {"Ακύρωση", "Κλείσιμο"},
	0x0a: {"Cancelar", "Cerrar"}, 0x0b: {"Peruuta", "Sulje"}, 0x0c: {"Annuler", "Fermer"}, 0x0d: {"ביטול", "סגירה"},
	0x0e: {"Mégse", "Bezárás"}, 0x10: {"Annulla", "Chiudi"}, 0x11: {"キャンセル", "閉じる"}, 0x12: {"취소", "닫기"},
	0x13: {"Annuleren", "Sluiten"}, 0x14: {"Avbryt", "Lukk"}, 0x15: {"Anuluj", "Zamknij"}, 0x16: {"Cancelar", "Fechar"},
	0x18: {"Anulează", "Închide"}, 0x19: {"Отмена", "Закрыть"}, 0x1a: {"Odustani", "Zatvori"}, 0x1b: {"Zrušiť", "Zavrieť"},
	0x1d: {"Avbryt", "Stäng"}, 0x1e: {"ยกเลิก", "ปิด"}, 0x1f: {"İptal", "Kapat"}, 0x21: {"Batal", "Tutup"},
	0x22: {"Скасувати", "Закрити"}, 0x24: {"Prekliči", "Zapri"}, 0x27: {"Atšaukti", "Užverti"}, 0x29: {"لغو", "بستن"}, 0x2a: {"Hủy", "Đóng"},
	0x281a: {"Откажи", "Затвори"}, 0x0c1a: {"Откажи", "Затвори"}, 0x241a: {"Otkaži", "Zatvori"}, 0x081a: {"Otkaži", "Zatvori"},
	0x0404: {"取消", "關閉"}, 0x0c04: {"取消", "關閉"}, 0x1404: {"取消", "關閉"},
}

func installerWords(m Manifest, language uint16) dialogWords {
	text := catalogWords(m, language)
	buttons := [2]string{"Cancel", "Close"}
	if text != english {
		var ok bool
		buttons, ok = buttonTranslations[language]
		if !ok {
			buttons, ok = buttonTranslations[language&0x3ff]
		}
		if !ok {
			buttons = [2]string{"Cancel", "Close"}
		}
	}
	return dialogWords{text, buttons[0], buttons[1]}
}
func displayName(m Manifest, language uint16) string {
	for _, name := range m.Names {
		if name.Language == language {
			return name.Text
		}
	}
	for _, name := range m.Names {
		if name.Language&0x3ff == language&0x3ff {
			return name.Text
		}
	}
	return m.Name
}
