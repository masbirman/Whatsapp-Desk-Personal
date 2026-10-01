# Repository Audit — WhatsApp Desk Personal

Audit statis atas checkout `main` pada 26 September 2026. Sumber utama adalah kode aplikasi, dependensi yang di-vendor, skrip build, dan tes di repository ini. Aplikasi tidak dijalankan, dependency tidak dipasang, dan build/tes tidak dieksekusi pada fase ini. Perilaku runtime yang bergantung pada versi WhatsApp Web, desktop environment, izin OS, atau hasil instalasi disebut sebagai belum terverifikasi.

## 1. Executive Summary

Repository berisi klien desktop WhatsApp Web berbasis Go. Jendela native memuat `https://web.whatsapp.com`; mayoritas fitur tambahan adalah JavaScript dan CSS dalam string besar `getInitScript` di `main.go`, yang dipasang sebelum navigasi. Kode native dipisahkan menurut platform melalui build tags: WebKitGTK di Linux, WebView2 di Windows, dan WKWebView di macOS. Bridge `Bind` menghubungkan halaman dengan notifikasi, penyimpanan file, pengaturan, updater, badge, dan operasi jendela.

Risiko utama untuk pengembangan berikutnya ialah ketergantungan luas pada DOM WhatsApp yang tidak dikendalikan project, bridge native yang tersedia dari konteks halaman, serta updater yang **masih menunjuk dan hanya menerima aset rilis upstream** (`vianziro/Whatsapp-Dekstop`), meskipun checkout ini adalah fork. “App lock” yang ada adalah blur otomatis pada idle/blur jendela, tanpa PIN, kata sandi, enkripsi, atau kunci akses native. Tidak ditemukan tray Windows dalam source aplikasi.

Rujukan: `app_config.go:5-8`, `main.go:13-25,1740-2170,4322-4341`, `app_linux.go:710-1008`, `app_windows.go:626-904`, `app_darwin.go:1172-1434`, `updater.go:30-32,475-492`.

## 2. Technology Stack

| Komponen | Implementasi aktual |
|---|---|
| Bahasa | Go 1.26 (`go.mod`); JavaScript dan CSS tertanam dalam Go; C/cgo GTK+GIO di Linux; Objective-C/cgo Cocoa di macOS; Win32/COM di Windows. |
| WebView | `github.com/webview/webview_go` di Linux/macOS; `github.com/jchv/go-webview2` di Windows. Keduanya berada dalam `vendor/`. |
| Runtime web | WebKitGTK 4.0 atau 4.1 pada build Linux, Edge WebView2 pada Windows, WKWebView pada macOS. |
| Integrasi tambahan | `github.com/go-toast/toast` untuk Windows Toast, `golang.org/x/sys/windows` untuk Win32, GLib/GDBus untuk Linux tray, Cocoa/UserNotifications/AVFoundation/PDFKit untuk macOS. |
| Dokumen | SheetJS minified di `assets/xlsx.core.min.js`, ditanam oleh `xlsx_asset.go`; pemrosesan format lain sebagian dibuat langsung di script `main.go`. |

`go.mod` memberi label `// indirect` pada seluruh requirement, namun kode platform mengimpor library tersebut secara langsung. `go.sum` dan `vendor/` tersedia; tidak ada package manager JavaScript aplikasi atau Electron. Rujukan: `go.mod`, `xlsx_asset.go`, import dan build tags di `app_*.go`.

Perbedaan dokumentasi/source yang relevan: README menyebut validasi `SHA256SUMS` sebelum update (`README.md:44`), tetapi source memperbolehkan file checksum yang **tidak ada** karena `checksumVerificationRequired = false`; jika checksum tersedia namun berbeda, update gagal (`checksum.go:19-27,153-178`). README menyebut “Auto-Lock” (`README.md:40`), sedangkan source menunjukkan hanya blur visual tanpa autentikasi (`main.go:2111-2163`).

## 3. Repository Structure

| Path | Tanggung jawab |
|---|---|
| `main.go` | `main()`, `WindowState`, generator satu script injeksi besar, UI/handler fitur dalam halaman. |
| `app_config.go` | Judul jendela dan URL WhatsApp Web. |
| `app_linux.go`, `app_windows.go`, `app_darwin.go` | Pembuatan WebView, bridge native, integrasi OS, lifecycle jendela, notifikasi, startup, badge. |
| `settings.go`, `settings_{linux,windows,darwin}.go` | `settings.json`, download dan validasi path, folder picker per platform. |
| `updater*.go`, `checksum.go` | Deteksi rilis, unduh dan terapkan update per platform, checksum aset. |
| `cache_budget*.go` | Batas disk cache 512 MiB dan penanganan cache aktif. |
| `debuglog.go`, `crashlog.go` | Diagnostik lokal opsional dan crash log. |
| `onboarding.go` | Script overlay pengenalan pertama kali. |
| `assets/`, `icon.*`, `AppIcon.icns`, `resource.rc`, `rsrc_windows_amd64.syso`, `app.manifest` | Aset dokumen, icon, dan resource platform. |
| `build*.sh`, `release.sh`, `make_checksums.sh`, `installer/` | Build, paket, installer, dan rilis. |
| `.github/workflows/build.yml` | Workflow rilis manual. |
| `*_test.go`, `testdata/init_script_harness.js` | Unit/guard tests dan harness JavaScript. |
| `site/`, `screenshots/`, `README.md`, `CHANGELOG.md` | Situs dan materi penjelasan; tidak digunakan sebagai bukti perilaku bila source berbeda. |
| `vendor/` | Source dependency yang dilacak repository. |

## 4. Application Startup Flow

`main()` memasang `recover` untuk panic pada goroutine utama, lalu memanggil `runApp()` terpilih oleh build tag (`main.go:4334-4341`). Masing-masing `runApp` menegakkan satu instance, menentukan lokasi data, menyiapkan cache/jendela, memuat pengaturan dan integrasi OS, mendaftarkan bridge `Bind`, lalu memanggil `w.Init(getInitScript(userAgent))`, `w.Navigate(appURL)`, memulai pengecekan update berkala dan `w.Run()` (`app_linux.go:710-1008`, `app_windows.go:626-904`, `app_darwin.go:1172-1434`).

`getInitScript` membangun satu script dari banyak modul `waRunModule`; error sinkron satu modul dicatat di `waRecoverable` agar modul berikutnya tetap terpasang (`main.go:13-84`). Script mengganti User-Agent, sebagian UA Client Hints, `navigator.vendor`, dan menyediakan `window.chrome` tiruan (`main.go:87-175`). Onboarding disisipkan dari `onboarding.go` di akhir script. Detail urutan DOM aktual saat WhatsApp Web memuat halaman belum diuji dalam audit statis ini.

## 5. WebView Architecture

**Linux.** `webview.New(false)` dari `webview_go`, yang pada build Linux memakai WebKitGTK; `app_linux.go` menambah GTK/GIO cgo (`app_linux.go:1-6,753-765`; `vendor/github.com/webview/webview_go/webview.go:7-9`). Source vendor membuat `webkit_web_view_new()` dan memasang `window.external.invoke` melalui WebKit message handler (`vendor/github.com/webview/webview_go/libs/webview/include/webview.h:1258-1279`). Tidak ada `DataPath` WebView eksplisit di `app_linux.go`.

**Windows.** `webview2.NewWithOptions` diberi `DataPath: getUserDataDir()` dan `Debug: false`; kunci accelerator WebView2 dinonaktifkan supaya shortcut halaman menerima F5/Ctrl+R/dll (`app_windows.go:653-682`). Aplikasi memakai WebView2 terpisah dari implementasi `webview_go` Windows di vendor.

**macOS.** `webview.New(false)` membuat WKWebView. `app_darwin.go` menemukan instance WKWebView dari jendela, menetapkan custom User-Agent, dukungan media inline, delegate izin media, serta pengaturan cache (`app_darwin.go:287-373,1172-1206`). Source vendor membuat `WKWebViewConfiguration` dan `WKWebView` tanpa memilih nonpersistent store (`vendor/.../webview.h:1924-1955`); lokasi profil dan ketahanan login di macOS tetap bergantung pada default WebKit dan belum diuji runtime.

Ketiganya menggunakan `Bind` sebagai fungsi Promise dari JavaScript ke Go, `Init` untuk script awal, `Navigate` untuk URL WhatsApp, dan `Eval` yang di-dispatch ke UI thread untuk banner update. Kode bridge hampir sama tetapi diduplikasi per file platform.

## 6. WhatsApp Web Integration

Halaman dimuat dari konstanta `appURL = "https://web.whatsapp.com"` (`app_config.go:5-8`). Aplikasi tidak menyediakan backend pesan/chat sendiri dan tidak ditemukan pemanggilan API privat WhatsApp. Login adalah UI WhatsApp Web dalam WebView. Interaksi tambahannya meliputi:

- Injeksi script dan style untuk privacy, layout, tema, settings, toast, onboarding, drag/drop, preview, badge unduhan, dan spell check (`main.go:13-4320`, `onboarding.go`). CSS dibuat sebagai elemen `<style>` atau `style.cssText` dalam script; tidak ada file stylesheet runtime terpisah.
- Intersepsi `window.Notification` dan `ServiceWorkerRegistration.prototype.showNotification` menuju `sendNativeNotification`; `Notification.permission` dilaporkan `granted` (`main.go:178-249`).
- Intersepsi klik/link eksternal, `window.open`, `URL.createObjectURL`, `HTMLAnchorElement.prototype.click`, dan drag/drop/file input untuk download, preview, dan upload (`main.go:350-950,1431-1523,2723-3033`).
- `MutationObserver` untuk media yang muncul, judul/badge unread, viewer dokumen, badge file tersimpan, spell check, serta pemasangan tombol Settings/tema pada UI WhatsApp yang berubah (`main.go:287-340,1546-1570,2511-2592,3005-3160,3163-3505`). Ada throttling/`requestAnimationFrame`/pause saat scroll atau halaman tersembunyi, tetapi observer tertentu tetap mencakup `document.body`.
- Bridge native untuk menyimpan/membuka file, mengubah pengaturan, OS notification, update, badge, tema, startup, always-on-top dan state jendela (`app_*.go`, blok `Bind`).

**Deteksi data WhatsApp:** unread hanya dibaca dari pola tanda kurung pada `document.title` (`main.go:1546-1570`), bukan dari model pesan. Nama dokumen diperkirakan dari `title`, `aria-label`, atau teks pendek di sekitar elemen pesan (`main.go:535-584`). Daftar chat, bubble dan media dikenali lewat struktur/selector DOM, bukan API kontak/chat/media yang stabil. Tidak ditemukan modul yang mengumpulkan daftar kontak, isi chat, atau pesan secara terstruktur untuk aplikasi native. Logic DOM terkonsentrasi secara fisik di `main.go`, tetapi tersebar di banyak modul JavaScript dalam file itu.

## 7. Existing Privacy Mode

**File/alur:** `main.go:1740-2110` membuat CSS `whatsapp-privacy-style`, menambah/menghapus kelas `privacy-mode` pada `<html>`, lalu menampilkan toast saat toggle manual. Shortcut Cmd/Ctrl+Shift+P dan Control Center memanggil `window.togglePrivacyMode`. Menu/status item macOS juga memanggil fungsi halaman lewat `MenuBridge` (`app_darwin.go:680-840`).

**State:** `isPrivacyActive` dalam memori halaman; `blur-avatars` adalah kelas terpisah yang nilainya dibaca/disimpan melalui `getBlurAvatarsNative`/`setBlurAvatarsNative` ke `settings.json` (`main.go:2059-2110`, `settings.go:21-33,182-195`). Toggle privacy manual sendiri tidak terlihat disimpan ke disk, sehingga reload dapat mengembalikannya ke nonaktif.

**Cakupan:** nama, preview, timestamp daftar chat, bubble teks/media, header percakapan, avatar opsional, viewer media, dengan reveal berdasarkan hover. Implementasi mencoba beberapa selector lama/baru serta label arsip (`main.go:1751-2028`). Blur adalah efek visual CSS; isi DOM, clipboard, notifikasi, dan bridge native tidak dikunci. Perubahan DOM/class WhatsApp dapat membuka area yang tidak lagi cocok dengan selector atau menghilangkan hover reveal. Modifikasi CSS/selector harus diuji pada beberapa bentuk daftar chat dan viewer.

## 8. Existing App Lock

Istilah di UI “auto-lock” berarti privacy blur otomatis: `AUTO_LOCK_KEY = wa_desk_privacy_autolock` disimpan lewat wrapper `localStorage`; timer 60 detik aktivitas browser, `blur` jendela, atau `visibilitychange` mengaktifkan privacy, lalu interaksi/focus memanggil `unlockFromIdle` tanpa autentikasi (`main.go:2111-2163`). Default adalah mati jika key belum berisi `1`. State yang relevan ialah `autoLockEnabled`, `idleTimer`, `autoLocked`, dan `isPrivacyActive`, semuanya di halaman. Seluruh OS memakai event WebView yang sama; tidak ditemukan API idle OS khusus Linux/Windows/macOS.

**Keterbatasan/risiko:** ini bukan app lock keamanan; aktivitas pertama langsung menghilangkan blur. `togglePrivacyMode` manual saat `autoLocked` hanya membalik `isPrivacyActive` dan tidak mengubah `autoLocked`, sehingga kombinasi manual/otomatis perlu diuji sebelum diubah. File `app.lock` Linux dan `whatsapp.lock` macOS (`app_linux.go:487-500`, `app_darwin.go:1056-1069`) adalah flock untuk satu instance, sama sekali bukan pengunci akses pengguna. Windows memakai mutex (`app_windows.go:404-417`).

## 9. Existing Notification System

Script mengganti konstruktor Notification dan `showNotification` service worker sehingga title/body dikirim ke bridge native bila setting siap dan aktif; setting di `AppSettings.NotificationsEnabled`, default `true` (`main.go:178-249`, `settings.go:21-33,62-111`). Control Center memanggil toggle JS lalu native (`main.go:3579-3593,3978-4002`). `window.Notification` tiruan menyediakan properti callback tetapi source ini tidak memasang perilaku klik/close/error yang setara API browser; kompatibilitas dengan seluruh pola pemanggilan WhatsApp belum terverifikasi.

| Platform | Native path dan batasan |
|---|---|
| Linux | `notify-send -a ... -i ...` via `exec.Command`; bergantung pada utilitas dan notification daemon. Error diabaikan (`app_linux.go:521-527`). |
| Windows | `github.com/go-toast/toast` mengirim Windows toast ke Notification Center dengan AppID dan activation arguments; hasil `Push` diabaikan (`app_windows.go:440-450`). Perilaku aktivasi pada instalasi nyata belum diuji. |
| macOS | `UNUserNotificationCenter` meminta otorisasi bila ada app bundle identifier, mengirim banner/sound dan membawa jendela depan saat diklik; eksekusi `go run` tanpa bundle tidak mengirim notifikasi (`app_darwin.go:173-235`). |

Audio mute (`main.go:2230-2257`) hanya mengubah elemen `audio, video` dan event `play`; tidak sama dengan toggle native notification dan tidak mematikan suara notifikasi macOS yang ditetapkan oleh `UNNotificationSound`.

## 10. Existing Tray Integration

**Linux:** implementasi StatusNotifierItem melalui GDBus di `app_linux.go:123-461`. State C meliputi koneksi DBus, visibilitas, overlay name dan `g_unread_count`. `Activate` menampilkan jendela; `SecondaryActivate`, `ContextMenu`, `Scroll` hanya mengembalikan sukses. Tidak ada menu tray. `updateDockBadge` mengubah overlay name dan tooltip; apakah host tray menampilkan nama overlay tersebut belum diuji (`app_linux.go:550-558,797-808`). Bergantung pada session bus dan StatusNotifierWatcher.

**Windows:** tidak ditemukan pembuatan notification area icon atau tray menu di `app_windows.go`; `updateDockBadge` memakai `ITaskbarList3` untuk taskbar progress sebagai indikasi unread, bukan angka tray (`app_windows.go:99-193,868-875`).

**macOS:** `NSStatusItem` dengan menu Show Window, Settings, tema, privacy, always-on-top, mute, downloads, update, reload, quit (`app_darwin.go:745-840`). Menu mengandalkan `MenuBridge` yang mengeksekusi fungsi JS pada jendela (`app_darwin.go:680-743`). Dock badge terpisah memakai `setDockBadge` (`app_darwin.go:393-400,1246-1252`). Mengubah cara jendela ditutup harus mempertimbangkan status item/menu agar app tetap dapat dibuka.

## 11. Existing Shortcut System

Sebagian besar shortcut adalah listener `keydown` pada halaman yang disuntikkan: privacy Cmd/Ctrl+Shift+P, always-on-top +T, mute +M, startup +S, update +U, onboarding +H, Settings Cmd/Ctrl+, dan folder download Cmd/Ctrl+Shift+D; zoom +/−/0, reload Cmd/Ctrl+R/F5 dan hard refresh +Shift+R, Escape untuk menutup chat dalam kondisi tertentu (`main.go:1524-1545,2164-2470,4137-4225`, `onboarding.go:98-108`). Kombinasi yang tampil di UI diverifikasi terhadap handler, bukan diasumsikan dari README. Windows perlu `SetBrowserAcceleratorKeysEnabled(false)` supaya key mencapai halaman (`app_windows.go:673-680`). macOS menu memiliki aksi, namun komentar source menyebut shortcut utama berada di script halaman (`app_darwin.go:900-935`). Tidak ditemukan global OS hotkey yang aktif ketika jendela tidak fokus. Perubahan shortcut berisiko konflik dengan WhatsApp dan WebView.

## 12. Local Storage / Settings

`settings.json` di `os.UserConfigDir()/WhatsAppDesk` pada Linux/Windows dan `~/Library/Application Support/WhatsAppDesk` pada macOS menyimpan folder download, notifikasi, tema, pengorganisasian per bulan, spell check, blur avatar, dan `LastCrashNotified` (`settings.go:21-100`). Source membaca ulang file untuk sebagian getter/setter; penulisan via `os.WriteFile` bukan transaksi atomik. Wrapper `storageGet/Set/Remove` pada script memakai origin `localStorage`, dengan fallback memori ketika storage ditolak (`main.go:27-59`). Key auto-lock ada di sana; `sessionStorage` dipakai untuk dismissal banner update (`main.go:2298-2305`), dan onboarding memakai key `whatsapp_desktop_onboarded_v4` (`onboarding.go:5-14`). `window_state.json` dan lock satu instance berada di `getUserDataDir()` masing-masing platform.

**Session/login:** tidak ditemukan penyimpanan token WhatsApp atau kode login sendiri. Windows memberi profil persisten WebView2 lewat `DataPath` (`app_windows.go:419-429,653-667`; `vendor/github.com/jchv/go-webview2/pkg/edge/chromium.go:78-94`). Linux membuat WebKitWebView default; macOS membuat WKWebView dengan konfigurasi default (`vendor/github.com/webview/webview_go/libs/webview/include/webview.h:1258-1279,1924-1955`). Dengan demikian sesi bergantung pada website data store WebView, bukan `settings.json`; perilaku bertahan-login sesudah restart dan lokasi tepat data WebKit Linux/macOS belum diverifikasi tanpa runtime. Cache budget berusaha menghapus cache HTTP, bukan cookie/localStorage/IndexedDB (`cache_budget.go`, `app_darwin.go:120-148`), tetapi efeknya pada sesi aktual belum diuji.

Download disimpan ke `~/Downloads/WhatsApp Downloads` secara default; opsi subfolder bulanan ada (`settings.go:35-40,140-160`). Preview dapat menulis file sementara ke `WhatsAppDeskPreview` (`settings.go:498-522`). JS memakai `fetch` → `Blob` → Data URI → bridge Go; batas ukuran 1 GiB, sanitasi basename, penolakan direktori sensitif, dan deduplikasi berdasarkan SHA-256 ada pada saver (`main.go:2758-2855`, `settings.go:238-481`). Biaya memori tetap tinggi karena beberapa salinan payload dalam JS/Go.

## 13. Platform-Specific Implementations

| Aspek | Linux | Windows | macOS |
|---|---|---|---|
| WebView | WebKitGTK 4.0/4.1, GTK3 (`app_linux.go:1-6,753-765`) | WebView2 dengan profil `DataPath` (`app_windows.go:653-682`) | WKWebView + Cocoa (`app_darwin.go:287-373,1172-1206`) |
| Notifikasi | `notify-send` | Windows Toast | UserNotifications + izin OS |
| Tray/badge | GDBus SNI tanpa menu; unread tooltip/overlay | Tidak ada tray; taskbar `ITaskbarList3` progress | NSStatusItem/menu dan Dock badge |
| Auto-lock/idle | Event JS halaman, tanpa API idle GTK | Event JS halaman, tanpa API idle Win32 | Event JS halaman, tanpa API idle Cocoa |
| Startup login | `~/.config/autostart/whatsapp-desk.desktop` (`app_linux.go:560-599`) | `HKCU\...\Run` melalui `reg` (`app_windows.go:216-241`) | `~/Library/LaunchAgents/com.whatsapp.desk.plist` via `launchctl` (`app_darwin.go:1085-1145`) |
| Jendela | GTK move/resize, monitor mapping; Wayland menolak posisi absolut (`app_linux.go:601-708`) | Win32 placement/maximized per monitor, DWM titlebar, always-on-top (`app_windows.go:360-625`) | NSWindow delegate close-to-hide, menu reopen, frame per layar, PDFKit (`app_darwin.go:156-203,991-1055`) |
| Satu instance | `flock` pada `app.lock` | named mutex, bawa jendela lama ke depan | `flock` pada `whatsapp.lock`, `osascript` activate |
| Media/native | GTK folder picker dan file manager; WebKitGTK | Windows folder picker, Win32 job object, working-set trim | AVFoundation camera/mic permission UI; native PDFKit preview, Cocoa menu |

Linux bergantung pada host StatusNotifierWatcher untuk tray dan `notify-send` untuk notifikasi. Windows memerlukan WebView2 Runtime. macOS memerlukan app bundle dengan identifier agar UserNotifications berfungsi. Semua dependency platform ini teridentifikasi dari source/build script, tetapi belum divalidasi pada mesin target.

Jejak fitur lintas platform yang penting:

| Fitur | File dan state utama | Dependency, batasan, risiko perubahan |
|---|---|---|
| Startup behavior | `main.go:4334-4341`; tiga `runApp`; one-instance lock/mutex; `onboarding.go` memakai key localStorage | Urutan `Bind` sebelum `Init/Navigate` diperlukan; error profil/WebView dapat menghentikan startup. Onboarding bergantung pada akses storage/DOM. |
| Session persistence | `app_windows.go:653-667` memberi `DataPath`; Linux/macOS memakai default WebKit dari `vendor/` | Tidak ada kode login khusus. Mengganti profil/store atau purge cache berisiko logout; hasil restart sesungguhnya belum diuji. |
| Window state | `WindowState` (`main.go:4322-4332`), resize bridge (`main.go:1590-1604`), `window_state.json` per platform | State disimpan ke user data dir, di-restore menurut monitor; Wayland hanya dapat menjamin ukuran. macOS close-to-hide berbeda dari Windows/Linux. |
| Native platform | `app_linux.go`, `app_windows.go`, `app_darwin.go` | Bridge yang sama harus tersedia di setiap platform. API GTK/GDBus, Win32/COM/WebView2, dan Cocoa/WebKit tidak saling menggantikan. |

## 14. DOM Dependency Analysis

Tidak ditemukan abstraction layer terpisah yang memetakan versi DOM WhatsApp ke interface stabil. Ada helper lokal per fitur dan fallback beberapa selector, misalnya `findAttachButton`, `findMediaInput`, `findDocumentInput`, `extractDocumentName`, `findVisibleViewerDownloadControl`, `privacyChatRowFromTarget`, dan logic mount Settings (`main.go:660-865,535-584,1980-2028,2840-2918,3327-3505`). Fallback ini mengurangi kegagalan untuk variasi DOM yang sudah dikenal, namun tidak memusatkan kontrak DOM.

Contoh dependensi yang perlu dipantau:

| Domain | Selector/heuristik aktual | Risiko perubahan |
|---|---|---|
| Daftar chat/privacy | `#side`, `#pane-side`, `[data-testid="chat-list"]`, `[role="row"]`, `._ak8q`, `._ak8h`, beberapa class `x...` (`main.go:1751-2028`) | Blur/hover dapat meleset atau terlalu luas. |
| Pesan/media | `#main`, `.message-in/out`, `[data-testid="msg-container"]`, `[data-testid="media-viewer"]` (`main.go:453-633,1781-1816`) | Preview, Escape, privacy dapat salah sasaran. |
| Upload | `data-testid` attach, `aria-label` multibahasa, `input[type=file]`, `accept` (`main.go:731-865`) | Salah memilih input atau gagal upload. |
| Download/dokumen | Tombol `download`/`Unduh`, icon, title/aria/text dan pola ekstensi (`main.go:535-584,2840-3033`) | Salah mengidentifikasi dokumen, memicu dua unduhan, atau tidak membuka preview. |
| UI Settings/tema | Header/rail WhatsApp, kelas `dark`, observer pada subtree (`main.go:3163-3505`) | Tombol hilang, duplikat, atau tema tidak sinkron. |
| Unread | `/\(([^)]+)\)/` pada judul halaman (`main.go:1546-1570`) | Format title lain menghasilkan badge keliru/kosong. |

Tidak ditemukan deteksi kontak/chat/message/media melalui model data WhatsApp. Deteksi tersebut adalah heuristik UI untuk elemen yang terlihat dan niat klik. `MutationObserver` tidak berarti data yang tidak ter-render dalam daftar virtual tersedia.

## 15. Fragile Areas

1. `getInitScript` sekitar 4.300 baris di `main.go` menggabungkan banyak modul dalam satu string Go. Perubahan kutip/backtick, cakupan variabel, urutan inisialisasi, atau monkey patch prototype dapat mematikan fitur halaman; `waRunModule` hanya mengisolasi error sinkron (`main.go:13-84`).
2. Selector class WhatsApp yang tampak terobfuskasi (`._ak8*`, `x...`) dan `data-testid` dapat berubah kapan pun; khususnya privacy, settings mount, upload, viewer, saved badges (`main.go:1740-2028,2723-3505`).
3. Observer body dan click listener capture dapat bersaing dengan UI WhatsApp; source sudah memakai throttling, tetapi performa dan kompatibilitas UI baru belum terverifikasi (`main.go:130-145,3005-3033`).
4. Hook `window.Notification`, `window.open`, `URL.createObjectURL`, `HTMLAnchorElement.prototype.click` mengubah API global halaman. WhatsApp dapat mengubah cara memanggilnya (`main.go:178-249,1431-1523,2918-2949`).
5. Badge unread hanya memakai title; Windows progress tidak menunjukkan angka dan Linux overlay tergantung tray host (`main.go:1546-1570`, `app_windows.go:177-193`, `app_linux.go:352-370`).
6. Duplikasi bridge lintas tiga platform memungkinkan perbedaan dukungan saat menambah fungsi baru; contoh `getAutoStartNative` ada di Windows/macOS tetapi tidak ditemukan di Linux (`app_linux.go:814-818`, `app_windows.go:740-747`, `app_darwin.go:1259-1266`).
7. `saveSettings` menulis file JSON langsung dan banyak pemanggil mengabaikan error; gangguan disk/izin dapat membuat UI menampilkan state yang tampak tersimpan padahal belum (`settings.go:94-111`, handler `Bind` platform).
8. Rilis fork berbeda dari upstream, sedangkan `githubRepo` dan allowlist updater tetap upstream. Menjalankan update dapat memasang binary upstream pada instalasi fork (`updater.go:30-32,253-335,475-560`). Ini temuan audit; tidak diubah di fase ini.

## 16. Reusable Components

`settings.go` menyediakan penyimpanan setting, validasi folder, saver download, deduplikasi, dan pembatasan open-file yang bisa dipakai fitur lanjutan. `checksum.go` dan `updater.go` menyediakan alur update yang sudah diuji unit tetapi perlu keputusan identitas rilis fork. `cache_budget.go` membatasi pertumbuhan cache; `debuglog.go`/`crashlog.go` memberi diagnostik lokal. `waRunModule`, wrapper storage, `showFloatingToast`, `escapeHtml`, serta `sanitizeSheetHtml` di `main.go` adalah helper yang dapat digunakan kembali dalam script yang sama (`main.go:27-84,370-409,1606-1725`). Bridge native `Bind` menyediakan pola integrasi lintas platform, meski belum diangkat menjadi satu interface terpusat.

## 17. Security Observations

- Script aplikasi berjalan di origin WhatsApp Web dan mempunyai akses ke fungsi bridge native. Nama file/teks chat yang masuk ke `innerHTML` diproses oleh `escapeHtml` atau sanitizer spreadsheet pada jalur yang tampak (`main.go:370-409,1139-1310`); perubahan UI yang menyisipkan data tak tepercaya perlu mempertahankan sanitasi itu.
- Saver menolak directory traversal lewat `filepath.Base`, memvalidasi folder download termasuk symlink sensitif, membatasi ukuran, dan deduplikasi berdasarkan isi. `openFileNative` dibatasi ke folder download dan preview (`settings.go:238-481,524-550`). Path yang ditentukan halaman tetap merupakan trust boundary; validasi saat membuka dan saat menulis harus dijaga.
- Link eksternal dikenali dengan `hostname.endsWith('whatsapp.com')` atau `.endsWith('whatsapp.net')` (`main.go:629-645,1503-1523`). Tanpa pemeriksaan batas label domain, hostname seperti `evilwhatsapp.com` juga cocok. Ini merupakan konsekuensi logis kondisi string di source, bukan temuan eksploit yang diuji.
- Updater membatasi URL awal ke `https://github.com/vianziro/whatsapp-dekstop/releases/...` dan membatasi unduhan 512 MiB (`updater.go:371-426,475-492`), tetapi `checksumVerificationRequired = false` membolehkan rilis lama tanpa `SHA256SUMS`; mismatch checksum tetap gagal (`checksum.go:22-27,153-178`). Release policy fork belum ada.
- Log debug lokal hanya aktif dengan `WA_DESK_DEBUG=1`; crash log memuat stack dan ringkasan panic, dibatasi/dirotasi; reporter membuat URL issue upstream setelah aksi pengguna (`debuglog.go:1-65`, `crashlog.go:18-120`, `main.go:1654-1739`). Karena URL issue dapat memuat potongan error/crash, pengguna harus meninjaunya sebelum mengirim.
- Privacy blur dan auto-lock tidak melindungi data dari script halaman, screenshot sebelum blur, pembaca file profil, atau pengguna yang berinteraksi dengan jendela; keduanya merupakan proteksi visual (`main.go:1740-2163`).

Audit ini tidak mencakup penetration test atau pemeriksaan dependency vulnerability terkini.

## 18. Test Coverage

Tes Go tersedia untuk settings/path validation dan deduplikasi, checksum, updater/platform asset selection/URL, crash log, serta banyak guard berbasis source-string untuk perilaku script UI (`settings_test.go`, `checksum_test.go`, `updater_test.go`, `updater_linux_test.go`, `crashlog_test.go`, `main_test.go`). `init_script_guard_test.go` memakai `testdata/init_script_harness.js` dan `jsdom` bila tersedia; detail dependency harness ada di file tersebut. `TestInjectedJavaScriptParses` dan tes lain memeriksa script, tetapi tidak sama dengan end-to-end pada WhatsApp Web live. `TestCheckForUpdateLive` ada (`updater_test.go:189`) dan berpotensi membutuhkan jaringan; tidak dijalankan.

Tes tidak dieksekusi pada audit ini sesuai batasan tidak memasang dependency/build. Coverage numerik dan hasil tes pada checkout ini belum terverifikasi. Tidak ditemukan suite end-to-end lintas tiga OS yang menjalankan login WhatsApp, notifikasi native, tray, atau perubahan DOM live.

## 19. Build and Release Process

`build.sh` menjadi wrapper untuk Windows/Linux/macOS dan pemeriksaan lokal; `build_windows.sh`, `build_windows_installer.sh`, `build_linux.sh`, `build_mac.sh` menghasilkan binary serta paket per platform. Linux memerlukan GTK3 dan development library WebKitGTK 4.0/4.1, `pkg-config`, cgo dan tool paket (deb/rpm sesuai target). `build_linux.sh` menyediakan alias `pkg-config` sementara untuk varian 4.1 (`build_linux.sh:1-105`). Windows memakai `go build` dengan `GOOS=windows`; `build.sh` dapat memilih CGO off tanpa MinGW, dan NSIS untuk Setup (`build.sh:20-63`, `installer/windows/WhatsAppDesk.nsi`). macOS memakai Go+cgo, framework Cocoa/WebKit/PDFKit/UserNotifications/AVFoundation, `lipo` untuk universal binary dan packaging DMG/ZIP (`app_darwin.go:1-8`, `build_mac.sh`).

`.github/workflows/build.yml` hanya dipicu `workflow_dispatch`, memvalidasi versi terhadap `updater.go`, membangun macOS, Windows, dan Linux x64/arm64, lalu menerbitkan GitHub Release dengan `SHA256SUMS` (`.github/workflows/build.yml:1-25,27-223`). Komentar workflow menyatakan pemicu tag otomatis dimatikan karena kondisi account upstream; keadaan account saat ini belum diverifikasi. `release.sh` adalah alur rilis lokal macOS+Windows, dengan `--dry-run` dan `--check`; Linux tidak masuk scope rilis lokal itu (`release.sh:1-17,80-143,192-236`). Semua skrip dan konstanta updater/rilis masih memakai identitas upstream di beberapa tempat; jangan menjalankannya untuk fork sebelum keputusan rilis.

## 20. Technical Constraints

- Fitur halaman bergantung pada DOM dan perilaku JavaScript WhatsApp Web yang berubah di luar repository.
- Tidak ada satu API aplikasi untuk data chat/kontak; interaksi terjadi lewat DOM yang ter-render.
- Backend dan izin native berbeda menurut OS; fitur bridge harus tersedia dan diuji pada tiga implementasi platform.
- WebView2, WebKitGTK, dan WKWebView memiliki perbedaan akselerator, notifikasi, media, cache, serta penempatan jendela.
- Payload lampiran melewati `Blob`/base64/Go string dalam memori; batas 1 GiB adalah batas penolakan, bukan penggunaan RAM maksimum (`settings.go:379-422`).
- Pembatasan cache 512 MiB melakukan penghapusan direktori cache saat melebihi budget; pemisahan data sesi harus dipertahankan (`cache_budget.go`, `app_darwin.go:120-148`).
- Versi Go dalam `go.mod` dan workflow adalah 1.26. Ketersediaan toolchain pada mesin target belum diperiksa.

## 21. Recommended Extension Points

Ini adalah peta titik integrasi hasil audit, bukan rencana implementasi. UI dalam halaman paling dekat dengan pola `waRunModule` di `getInitScript` dan helper storage/toast/sanitasi (`main.go:27-84,370-409,1606-1653`). Fitur yang memerlukan OS atau filesystem masuk melalui `Bind` di ketiga `app_*.go`, dengan fungsi bersama di file Go netral platform seperti `settings.go`. Preferensi yang harus bertahan lintas reload bisa memakai `AppSettings`; preferensi yang hanya terkait halaman saat ini memakai wrapper storage JS. Fitur yang menyentuh WhatsApp DOM sebaiknya memakai helper selector terlokalisasi dan fallback yang teruji, karena saat ini tidak ada abstraction layer khusus kompatibilitas DOM.

## 22. Areas That Should Not Be Modified Without Strong Reason

- Profil WebView/session dan jalur cache pada `app_*.go`, vendor WebView, dan `cache_budget*.go`: salah hapus dapat memutus login atau merusak media/cache.
- Pembatasan path/payload pada `settings.go` dan sanitasi HTML di `main.go`: ini menjaga bridge native dan UI dari input file/chat.
- URL allowlist, checksum, dan mekanisme terapkan update pada `updater*.go`/`checksum.go`: ini trust boundary untuk eksekusi binary baru; identitas fork tetap perlu keputusan tersendiri.
- Win32 job object, accelerator WebView2, dan lifecycle suspend pada `app_windows.go`: komentar source mencatat black screen/frozen renderer jika konfigurasi ini salah.
- Delegate close-to-hide, callback PDFKit, menu bridge, dan memory purge macOS di `app_darwin.go`: terkait lifecycle native dan WebView aktif.
- Hook API global dan event capture di `main.go`: perubahan kecil dapat memengaruhi perilaku dasar WhatsApp Web.
- `vendor/`: source dependency yang dipakai build; perubahan langsung sulit dipelihara dan perlu alasan yang dapat diuji.

## 23. Open Technical Questions

1. Pada build nyata Linux/macOS, di path mana default WebKit menyimpan cookies/IndexedDB dan apakah login bertahan pada semua format paket? Source tidak menetapkan lokasi profil khusus; butuh uji runtime.
2. Apakah fallback selector privacy, upload, viewer, dan Settings masih cocok dengan DOM WhatsApp Web versi saat audit? Tidak diuji ke halaman live.
3. Apakah semua bentuk notifikasi WhatsApp Web memakai `window.Notification` atau `ServiceWorkerRegistration.showNotification` yang diintersepsi? Tidak dapat dipastikan dari source aplikasi saja.
4. Apakah Linux tray overlay tampil pada desktop environment target, dan bagaimana perilaku saat tidak ada StatusNotifierWatcher? Belum diuji.
5. Apakah Windows Toast activation arguments membuka/fokus jendela sebagaimana dimaksud pada instalasi nyata? Belum diuji.
6. Identitas rilis, updater, issue reporter, nama aplikasi, profil, dan signature apa yang diinginkan untuk fork personal? Source masih memakai banyak nilai upstream; perlu spesifikasi fase berikutnya sebelum modifikasi.
7. Apakah `settings.json` perlu penulisan atomik dan propagasi error yang terlihat UI? Source saat ini sering mengabaikan error; dampak pada target filesystem belum diuji.
8. Apakah “app lock” untuk versi personal nantinya cukup blur otomatis atau memerlukan autentikasi? Implementasi saat ini hanya blur.

## 24. Conclusion

Aplikasi adalah shell native lintas platform untuk WhatsApp Web dengan fitur desktop yang terutama diimplementasikan sebagai script/CSS halaman, ditopang bridge Go untuk OS dan filesystem. Struktur Go platform cukup jelas, sedangkan ketergantungan DOM WhatsApp tersebar dalam satu script besar. Area paling penting untuk pengembangan berikutnya adalah `main.go` (injeksi dan DOM), tiga `app_*.go` (bridge/native), `settings.go` (persistensi dan keamanan file), dan `updater*.go`/`checksum.go` (identitas rilis fork). Semua temuan ini berasal dari pembacaan source; perilaku runtime lintas OS dan DOM WhatsApp live masih perlu diverifikasi pada fase terpisah.
