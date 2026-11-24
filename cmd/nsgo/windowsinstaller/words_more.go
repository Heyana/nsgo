package windowsinstaller

// The earlier Modern UI used English for its extra desktop/running prompts outside words.go's translated set. Retain that fallback on these pages too.
var standardTranslations = map[uint16][6]string{
	0x01:   {"تثبيت", "إلغاء التثبيت", "اختيار مجلد…", "فتح التطبيق", "اكتمل.", "يرجى الانتظار…"},
	0x02:   {"Инсталиране", "Деинсталиране", "Избор на папка…", "Отваряне на приложението", "Готово.", "Моля, изчакайте…"},
	0x03:   {"Instal·la", "Desinstal·la", "Tria una carpeta…", "Obre l’aplicació", "Completat.", "Espereu…"},
	0x05:   {"Nainstalovat", "Odinstalovat", "Vybrat složku…", "Otevřít aplikaci", "Dokončeno.", "Čekejte prosím…"},
	0x06:   {"Installer", "Afinstaller", "Vælg mappe…", "Åbn app", "Fuldført.", "Vent venligst…"},
	0x08:   {"Εγκατάσταση", "Απεγκατάσταση", "Επιλογή φακέλου…", "Άνοιγμα εφαρμογής", "Ολοκληρώθηκε.", "Περιμένετε…"},
	0x29:   {"نصب", "حذف نصب", "انتخاب پوشه…", "باز کردن برنامه", "انجام شد.", "لطفاً صبر کنید…"},
	0x0b:   {"Asenna", "Poista asennus", "Valitse kansio…", "Avaa sovellus", "Valmis.", "Odota…"},
	0x0d:   {"התקנה", "הסרת התקנה", "בחירת תיקייה…", "פתיחת היישום", "הושלם.", "נא להמתין…"},
	0x1a:   {"Instaliraj", "Deinstaliraj", "Odaberi mapu…", "Otvori aplikaciju", "Dovršeno.", "Pričekajte…"},
	0x0e:   {"Telepítés", "Eltávolítás", "Mappa kiválasztása…", "Alkalmazás megnyitása", "Kész.", "Kis türelmet…"},
	0x21:   {"Instal", "Hapus instalasi", "Pilih folder…", "Buka aplikasi", "Selesai.", "Harap tunggu…"},
	0x27:   {"Įdiegti", "Pašalinti", "Pasirinkti aplanką…", "Atverti programą", "Baigta.", "Palaukite…"},
	0x13:   {"Installeren", "Verwijderen", "Map kiezen…", "App openen", "Voltooid.", "Even geduld…"},
	0x14:   {"Installer", "Avinstaller", "Velg mappe…", "Åpne app", "Fullført.", "Vent litt…"},
	0x15:   {"Zainstaluj", "Odinstaluj", "Wybierz folder…", "Otwórz aplikację", "Ukończono.", "Czekaj…"},
	0x18:   {"Instalează", "Dezinstalează", "Alege dosarul…", "Deschide aplicația", "Finalizat.", "Așteptați…"},
	0x1b:   {"Nainštalovať", "Odinštalovať", "Vybrať priečinok…", "Otvoriť aplikáciu", "Dokončené.", "Čakajte…"},
	0x24:   {"Namesti", "Odstrani", "Izberi mapo…", "Odpri aplikacijo", "Dokončano.", "Počakajte…"},
	0x281a: {"Инсталирај", "Деинсталирај", "Изабери фасциклу…", "Отвори апликацију", "Завршено.", "Сачекајте…"},
	0x0c1a: {"Инсталирај", "Деинсталирај", "Изабери фасциклу…", "Отвори апликацију", "Завршено.", "Сачекајте…"},
	0x241a: {"Instaliraj", "Deinstaliraj", "Izaberi fasciklu…", "Otvori aplikaciju", "Završeno.", "Sačekajte…"},
	0x081a: {"Instaliraj", "Deinstaliraj", "Izaberi fasciklu…", "Otvori aplikaciju", "Završeno.", "Sačekajte…"},
	0x1d:   {"Installera", "Avinstallera", "Välj mapp…", "Öppna app", "Klart.", "Vänta…"},
	0x1e:   {"ติดตั้ง", "ถอนการติดตั้ง", "เลือกโฟลเดอร์…", "เปิดแอป", "เสร็จสิ้น", "โปรดรอ…"},
	0x1f:   {"Yükle", "Kaldır", "Klasör seç…", "Uygulamayı aç", "Tamamlandı.", "Lütfen bekleyin…"},
	0x22:   {"Встановити", "Видалити", "Вибрати папку…", "Відкрити застосунок", "Завершено.", "Зачекайте…"},
	0x2a:   {"Cài đặt", "Gỡ cài đặt", "Chọn thư mục…", "Mở ứng dụng", "Hoàn tất.", "Vui lòng đợi…"},
}

func moreWords(language uint16) (words, bool) {
	labels, ok := standardTranslations[language]
	if !ok {
		labels, ok = standardTranslations[language&0x3ff]
	}
	if !ok {
		return words{}, false
	}
	return words{labels[0], labels[1], labels[2], english.Desktop, labels[3], english.Running, labels[4], labels[5]}, true
}
func (w words) layoutFlag() uint32 {
	for _, r := range w.Install {
		if r >= 0x0590 && r <= 0x08ff {
			return 0x2000
		}
		break
	}
	return 0
}
