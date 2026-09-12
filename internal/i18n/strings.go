package i18n

import (
	"fmt"
	"time"
)

// catalog traduz rótulos curtos e fixos (telas, status, bandeja).
var catalog = map[string]map[Lang]string{
	// tela de música
	"music.now_playing": {PT: "TOCANDO AGORA", EN: "NOW PLAYING", ES: "SONANDO AHORA", JA: "再生中", ZH: "正在播放"},
	"music.idle":        {PT: "MÚSICA", EN: "MUSIC", ES: "MÚSICA", JA: "音楽", ZH: "音乐"},
	"music.paused":      {PT: "PAUSADO", EN: "PAUSED", ES: "PAUSADO", JA: "一時停止", ZH: "已暂停"},
	"music.nothing":     {PT: "Nada tocando", EN: "Nothing playing", ES: "Nada sonando", JA: "何も再生していません", ZH: "没有正在播放的内容"},
	"music.hint":        {PT: "Dê play no Spotify, YouTube…", EN: "Press play on Spotify, YouTube…", ES: "Reproduce algo en Spotify, YouTube…", JA: "Spotify や YouTube で再生してください…", ZH: "在 Spotify、YouTube 等应用中播放音乐…"},

	// tela de jogo
	"game.header":               {PT: "JOGANDO AGORA", EN: "NOW PLAYING", ES: "JUGANDO AHORA", JA: "プレイ中", ZH: "正在游戏"},
	"game.empty_title":          {PT: "Nenhum jogo aberto", EN: "No game open", ES: "Ningún juego abierto", JA: "起動中のゲームはありません", ZH: "没有正在运行的游戏"},
	"game.empty_detail":         {PT: "Abra um jogo pela Steam", EN: "Open a game through Steam", ES: "Abre un juego desde Steam", JA: "Steam でゲームを起動してください", ZH: "通过 Steam 打开一个游戏"},
	"game.notfound_title":       {PT: "Steam não encontrada", EN: "Steam not found", ES: "Steam no encontrada", JA: "Steam が見つかりません", ZH: "未找到 Steam"},
	"game.notfound_detail":      {PT: "Abra a Steam ou veja a aba Steam do painel", EN: "Open Steam, or check the Steam tab in the panel", ES: "Abre Steam o revisa la pestaña Steam del panel", JA: "Steam を起動するか、パネルの「Steam」タブを確認してください", ZH: "打开 Steam,或查看面板中的 Steam 选项卡"},
	"game.notconfigured_title":  {PT: "Steam não configurada", EN: "Steam not set up", ES: "Steam no configurada", JA: "Steam が設定されていません", ZH: "尚未配置 Steam"},
	"game.notconfigured_detail": {PT: "Coloque sua API key no painel", EN: "Add your API key in the panel", ES: "Ingresa tu API key en el panel", JA: "パネルで API キーを入力してください", ZH: "在面板中填写你的 API 密钥"},
	"game.total":                {PT: "TOTAL JOGADO", EN: "TOTAL PLAYED", ES: "TOTAL JUGADO", JA: "合計プレイ時間", ZH: "总游戏时长"},
	"game.twoweeks":             {PT: "ÚLT. 2 SEMANAS", EN: "LAST 2 WEEKS", ES: "ÚLT. 2 SEMANAS", JA: "過去2週間", ZH: "近2周"},
	"game.session_suffix":       {PT: "nesta sessão", EN: "this session", ES: "esta sesión", JA: "今回のセッション", ZH: "本次会话"},

	// tela de sistema
	"system.header":        {PT: "SISTEMA", EN: "SYSTEM", ES: "SISTEMA", JA: "システム", ZH: "系统"},
	"system.uptime_prefix": {PT: "ligado há ", EN: "up for ", ES: "encendido hace ", JA: "起動時間 ", ZH: "已开机 "},
	"system.disk":          {PT: "DISCO ", EN: "DISK ", ES: "DISCO ", JA: "ディスク ", ZH: "磁盘 "},
	"system.net":           {PT: "REDE", EN: "NETWORK", ES: "RED", JA: "ネットワーク", ZH: "网络"},

	// status da conexão com a tela (app.go)
	"app.starting":      {PT: "Iniciando…", EN: "Starting…", ES: "Iniciando…", JA: "起動中…", ZH: "正在启动…"},
	"app.connecting":    {PT: "Conectando na tela…", EN: "Connecting to the screen…", ES: "Conectando con la pantalla…", JA: "画面に接続しています…", ZH: "正在连接屏幕…"},
	"app.connected":     {PT: "Conectada em %s", EN: "Connected on %s", ES: "Conectado en %s", JA: "%s に接続しました", ZH: "已连接到 %s"},
	"app.simulated":     {PT: "Modo simulado: sem tela USB, só a prévia do painel", EN: "Simulated mode: no USB screen, only the panel preview", ES: "Modo simulado: sin pantalla USB, solo la vista previa del panel", JA: "シミュレーションモード:USB画面はなく、パネルのプレビューのみ", ZH: "模拟模式:没有 USB 屏幕,仅面板预览"},
	"app.port_busy":     {PT: "A porta da tela está sendo usada por outro programa", EN: "The screen's port is being used by another program", ES: "El puerto de la pantalla está siendo usado por otro programa", JA: "画面のポートが他のプログラムに使用されています", ZH: "屏幕端口正被其他程序占用"},
	"app.not_found":     {PT: "Tela não encontrada. Ela está plugada na USB?", EN: "Screen not found. Is it plugged into USB?", ES: "No se encontró la pantalla. ¿Está conectada por USB?", JA: "画面が見つかりません。USBに接続されていますか?", ZH: "未找到屏幕。是否已插入 USB?"},
	"app.lost_connection": {PT: "Perdi a conexão com a tela, reconectando…", EN: "Lost connection to the screen, reconnecting…", ES: "Se perdió la conexión con la pantalla, reconectando…", JA: "画面への接続が切れました。再接続しています…", ZH: "与屏幕的连接已断开,正在重新连接…"},

	// ícone da bandeja
	"tray.tooltip":         {PT: "Bifrost — tela USB", EN: "Bifrost — USB screen", ES: "Bifrost — pantalla USB", JA: "Bifrost — USB画面", ZH: "Bifrost — USB 屏幕"},
	"tray.open_panel":      {PT: "Abrir painel", EN: "Open panel", ES: "Abrir panel", JA: "パネルを開く", ZH: "打开面板"},
	"tray.open_panel_desc": {PT: "Configurar o que aparece na tela", EN: "Configure what shows on the screen", ES: "Configurar qué se muestra en la pantalla", JA: "画面に表示する内容を設定する", ZH: "配置屏幕上显示的内容"},
	"tray.next":            {PT: "Próxima tela", EN: "Next screen", ES: "Siguiente pantalla", JA: "次の画面", ZH: "下一个屏幕"},
	"tray.pause":           {PT: "Pausar tela", EN: "Pause screen", ES: "Pausar pantalla", JA: "画面を一時停止", ZH: "暂停屏幕"},
	"tray.pause_desc":      {PT: "Congela o que está sendo mostrado", EN: "Freezes what's currently shown", ES: "Congela lo que se está mostrando", JA: "現在表示されている内容を固定します", ZH: "冻结当前显示的内容"},
	"tray.reconnect":       {PT: "Reconectar", EN: "Reconnect", ES: "Reconectar", JA: "再接続", ZH: "重新连接"},
	"tray.reconnect_desc":  {PT: "Fecha e reabre a conexão com a tela", EN: "Closes and reopens the connection to the screen", ES: "Cierra y vuelve a abrir la conexión con la pantalla", JA: "画面への接続を閉じて再度開きます", ZH: "关闭并重新打开与屏幕的连接"},
	"tray.quit":            {PT: "Sair", EN: "Quit", ES: "Salir", JA: "終了", ZH: "退出"},
	"tray.quit_desc":       {PT: "Fecha o Bifrost", EN: "Closes Bifrost", ES: "Cierra Bifrost", JA: "Bifrost を終了します", ZH: "关闭 Bifrost"},
}

type units struct {
	day, hour, min string
	space          bool
}

var unitTable = map[Lang]units{
	PT: {"d", "h", "min", true},
	EN: {"d", "h", "min", true},
	ES: {"d", "h", "min", true},
	JA: {"日", "時間", "分", false},
	ZH: {"天", "小时", "分钟", false},
}

func (u units) sep() string {
	if u.space {
		return " "
	}
	return ""
}

// Playtime formata minutos como "9995h 47min" (com as unidades do idioma l).
func Playtime(l Lang, min int) string {
	u := unitTable[normalize(l)]
	if min <= 0 {
		return fmt.Sprintf("0%s", u.min)
	}
	h, m := min/60, min%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%d%s%s%d%s", h, u.hour, u.sep(), m, u.min)
	case h > 0:
		return fmt.Sprintf("%d%s", h, u.hour)
	}
	return fmt.Sprintf("%d%s", m, u.min)
}

// SessionText formata a duração da sessão atual, ex.: "1h 35min · this session".
func SessionText(l Lang, d time.Duration) string {
	return fmt.Sprintf("%s · %s", Playtime(l, int(d.Minutes())), T(l, "game.session_suffix"))
}

// Uptime formata há quanto tempo o PC está ligado, ex.: "1d 4h" ou "6h 03min".
func Uptime(l Lang, d time.Duration) string {
	u := unitTable[normalize(l)]
	h := int(d.Hours())
	if h >= 24 {
		return fmt.Sprintf("%d%s%s%d%s", h/24, u.day, u.sep(), h%24, u.hour)
	}
	return fmt.Sprintf("%d%s%s%02d%s", h, u.hour, u.sep(), int(d.Minutes())%60, u.min)
}

// AMPM devolve o sufixo de 12 horas (vazio nos idiomas/formatos que não usam).
func AMPM(l Lang, pm bool) string {
	switch l {
	case JA:
		if pm {
			return "午後"
		}
		return "午前"
	case ZH:
		if pm {
			return "下午"
		}
		return "上午"
	}
	if pm {
		return "PM"
	}
	return "AM"
}

var weekdaysTable = map[Lang][]string{
	PT: {"domingo", "segunda", "terça", "quarta", "quinta", "sexta", "sábado"},
	EN: {"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	ES: {"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"},
	JA: {"日曜日", "月曜日", "火曜日", "水曜日", "木曜日", "金曜日", "土曜日"},
	ZH: {"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"},
}

var monthsTable = map[Lang][]string{
	PT: {"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"},
	EN: {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	ES: {"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
}

// normalize cai para o inglês quando l não é um dos idiomas suportados.
func normalize(l Lang) Lang {
	if IsSupported(l) {
		return l
	}
	return EN
}

// DateLine formata a data por extenso, na ordem e gramática de cada idioma.
func DateLine(l Lang, t time.Time) string {
	l = normalize(l)
	wd := weekdaysTable[l][int(t.Weekday())]
	switch l {
	case EN:
		return fmt.Sprintf("%s, %s %d", wd, monthsTable[EN][t.Month()-1], t.Day())
	case ES:
		return fmt.Sprintf("%s, %d de %s", wd, t.Day(), monthsTable[ES][t.Month()-1])
	case JA:
		return fmt.Sprintf("%d月%d日(%s)", t.Month(), t.Day(), string([]rune(wd)[0]))
	case ZH:
		return fmt.Sprintf("%d月%d日 %s", t.Month(), t.Day(), wd)
	default: // PT
		return fmt.Sprintf("%s, %d de %s", wd, t.Day(), monthsTable[PT][t.Month()-1])
	}
}
