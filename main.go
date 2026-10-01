package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Lang string

const (
	LangUZ Lang = "uz"
	LangEN Lang = "en"
	LangRU Lang = "ru"
)

// FSM Steps
const (
	StepChooseLang = iota
	StepWaitingName
	StepWaitingContact
	StepWaitingProject
	StepWaitingTech
)

// UserState stores client state in RAM
type UserState struct {
	Language    Lang
	Step        int
	Name        string
	Phone       string
	Telegram    string
	ProjectInfo string
	Technology  string
	UserID      int64
	UpdatedAt   time.Time
}

var (
	userStates = make(map[int64]*UserState)
	statesMu   sync.RWMutex

	// Config
	botToken      string
	channelID     string
	adminChatID   int64
	port          string
	portfolioURL  string
	developerTG   = "@haydaraliakbarov"
	developerName = "Akbarov Haydarali"
	cfgMu         sync.RWMutex
)

func getTechLabels(l Lang) (react, next, vue string) {
	switch l {
	case LangEN:
		return "⚛️ React.js (For complex projects)",
			"▲ Next.js (For easy / fast projects)",
			"🟢 Vue.js (For easy / lightweight projects)"
	case LangRU:
		return "⚛️ React.js (Для сложных задач)",
			"▲ Next.js (Для простых задач)",
			"🟢 Vue.js (Для простых задач)"
	default:
		return "⚛️ React.js (Qiyin ishlar uchun)",
			"▲ Next.js (Oson ishlar uchun)",
			"🟢 Vue.js (Oson ishlar uchun)"
	}
}

func getLangName(l Lang) string {
	switch l {
	case LangEN:
		return "🇬🇧 English"
	case LangRU:
		return "🇷🇺 Русский"
	default:
		return "🇺🇿 O'zbekcha"
	}
}

func main() {
	log.Println("====================================================")
	log.Println("⚡️ Haydarali Akbarov — Telegram Bot & Backend (Go)")
	log.Println("====================================================")

	loadConfig()

	if botToken == "" {
		log.Fatal("❌ TELEGRAM_BOT_TOKEN topilmadi! .env yoki environment variablesni tekshiring.")
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("❌ Telegram Bot API ulanishda xatolik: %v", err)
	}

	log.Printf("✅ Bot muvaffaqiyatli ulandi: @%s (ID: %d)", bot.Self.UserName, bot.Self.ID)

	// Set bot commands menu
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "Start bot / Boshlash"},
		tgbotapi.BotCommand{Command: "lang", Description: "Tilni o'zgartirish / Change language / Сменить язык"},
		tgbotapi.BotCommand{Command: "order", Description: "Loyiha buyurtma berish / Order project / Заказать проект"},
		tgbotapi.BotCommand{Command: "channel", Description: "Kanal holatini tekshirish / Check channel status"},
		tgbotapi.BotCommand{Command: "portfolio", Description: "Portfolio / Портфолио"},
		tgbotapi.BotCommand{Command: "help", Description: "Yordam / Help / Помощь"},
		tgbotapi.BotCommand{Command: "cancel", Description: "Bekor qilish / Cancel / Отмена"},
	)
	_, _ = bot.Request(commands)

	// Check if Webhook or Long-Polling
	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		webhookURL = os.Getenv("RENDER_EXTERNAL_URL")
	}

	var updates tgbotapi.UpdatesChannel

	// 1. Root and healthcheck endpoint for Render (so it stays 200 OK)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/health" || r.URL.Path == "/api/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status":      "ok",
				"bot":         "@" + bot.Self.UserName,
				"service":     "portfolio-bot-multilingual",
				"languages":   []string{"uz", "en", "ru"},
				"time":        time.Now().Format(time.RFC3339),
				"channel_set": channelID != "",
			})
			return
		}
		http.NotFound(w, r)
	})

	// Order submission via website REST API
	http.HandleFunc("/api/order", func(w http.ResponseWriter, r *http.Request) {
		handleWebOrder(bot, w, r)
	})

	// 2. Setup Webhook or Polling
	if webhookURL != "" {
		webhookPath := "/webhook/" + botToken
		fullWebhookURL := strings.TrimRight(webhookURL, "/") + webhookPath

		log.Printf("🌐 Webhook arxitekturasi faollashmoqda: %s", fullWebhookURL)

		wh, err := tgbotapi.NewWebhook(fullWebhookURL)
		if err != nil {
			log.Fatalf("NewWebhook xatosi: %v", err)
		}
		_, err = bot.Request(wh)
		if err != nil {
			log.Fatalf("SetWebhook xatosi: %v", err)
		}

		info, err := bot.GetWebhookInfo()
		if err == nil {
			log.Printf("✅ Webhook muvaffaqiyatli sozlandi: %s (Kutilayotgan: %d)", info.URL, info.PendingUpdateCount)
		}

		updates = bot.ListenForWebhook(webhookPath)

		go func() {
			log.Printf("🚀 Webhook server 0.0.0.0:%s da tinglamoqda...", port)
			if err := http.ListenAndServe("0.0.0.0:"+port, nil); err != nil {
				log.Fatalf("HTTP server xatosi: %v", err)
			}
		}()

		// Keep-alive background ping so Render NEVER goes to sleep!
		go func() {
			time.Sleep(15 * time.Second)
			ticker := time.NewTicker(4 * time.Minute)
			for range ticker.C {
				resp, err := http.Get(strings.TrimRight(webhookURL, "/") + "/health")
				if err == nil {
					_ = resp.Body.Close()
					log.Println("💓 Keep-alive ping yuborildi (Render uxlab qolmaydi)")
				}
			}
		}()
	} else {
		log.Println("⚡️ RENDER_EXTERNAL_URL topilmadi. Lokal Long-Polling rejimida ishlamoqda...")
		_, _ = bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: false})

		u := tgbotapi.NewUpdate(0)
		u.Timeout = 30
		updates = bot.GetUpdatesChan(u)

		go func() {
			log.Printf("🚀 Lokal healthcheck server 0.0.0.0:%s da ishlamoqda...", port)
			_ = http.ListenAndServe("0.0.0.0:"+port, nil)
		}()
	}

	log.Println("🤖 Bot 3 tilda xabarlarni qabul qilishga tayyor (UZ, EN, RU)!")

	// 3. Process incoming updates
	for update := range updates {
		handleUpdate(bot, update)
	}
}

func handleUpdate(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	// 1. Channel Post (Auto-detect channel)
	if update.ChannelPost != nil && update.ChannelPost.Chat != nil {
		detectChannel(bot, update.ChannelPost.Chat)
		return
	}

	// 2. Bot added to channel/group as admin
	if update.MyChatMember != nil && (update.MyChatMember.Chat.IsChannel() || update.MyChatMember.Chat.IsGroup() || update.MyChatMember.Chat.IsSuperGroup()) {
		detectChannel(bot, &update.MyChatMember.Chat)
		return
	}

	// 3. Inline button callback
	if update.CallbackQuery != nil {
		handleCallback(bot, update.CallbackQuery)
		return
	}

	// 4. Regular message
	if update.Message != nil {
		handleMessage(bot, update.Message)
	}
}

func detectChannel(bot *tgbotapi.BotAPI, chat *tgbotapi.Chat) {
	if chat == nil {
		return
	}
	chID := fmt.Sprintf("%d", chat.ID)

	cfgMu.RLock()
	alreadyLinked := (channelID == chID)
	cfgMu.RUnlock()

	// If already linked, do not send confirmation again to avoid spamming the channel!
	if alreadyLinked {
		return
	}

	setChannelID(chID)
	log.Printf("📢 [KANAL ANIQLANDI] Nomi: '%s', Username: '%s', ID: %s", chat.Title, chat.UserName, chID)

	confirmText := fmt.Sprintf(
		"✅ <b>Haydarali Akbarov Portfolio Boti ulandi!</b> 🚀\n\n"+
			"📢 <b>Kanal:</b> %s\n"+
			"🆔 <b>Kanal ID:</b> <code>%s</code>\n\n"+
			"Ushbu kanal yangi loyiha takliflari va buyurtmalarni qabul qilish uchun muvaffaqiyatli sozlandi.",
		html.EscapeString(chat.Title),
		chID,
	)
	msg := tgbotapi.NewMessage(chat.ID, confirmText)
	msg.ParseMode = tgbotapi.ModeHTML
	_, _ = bot.Send(msg)

	cfgMu.RLock()
	adminID := adminChatID
	cfgMu.RUnlock()

	if adminID != 0 && adminID != chat.ID {
		adminNotice := fmt.Sprintf(
			"📢 <b>Taklif kanali ulandi!</b> 🚀\n\n"+
				"Nomi: <b>%s</b>\n"+
				"Kanal ID: <code>%s</code>\n\n"+
				"💡 <b>Doimiy saqlash:</b>\n"+
				"Render.com da <b>Environment</b> bo'limiga kirib:\n"+
				"<code>TELEGRAM_CHANNEL_ID</code> = <code>%s</code>\n"+
				"deb qo'shib qo'ysangiz, server qayta yonganda ham kanal uzilmaydi!",
			html.EscapeString(chat.Title),
			chID,
			chID,
		)
		adminMsg := tgbotapi.NewMessage(adminID, adminNotice)
		adminMsg.ParseMode = tgbotapi.ModeHTML
		_, _ = bot.Send(adminMsg)
	}
}

func showLanguageSelection(bot *tgbotapi.BotAPI, chatID int64) {
	text := "👋 <b>Assalomu alaykum! / Hello! / Здравствуйте!</b> 🚀\n\n" +
		"Men <b>Haydarali Akbarov</b>ning rasmiy portfolio botiman.\n\n" +
		"🌐 <b>Iltimos, muloqot tilini tanlang:</b>\n" +
		"Please choose your language:\n" +
		"Пожалуйста, выберите язык:"

	btnUZ := tgbotapi.NewInlineKeyboardButtonData("🇺🇿 O'zbekcha", "set_lang:uz")
	btnEN := tgbotapi.NewInlineKeyboardButtonData("🇬🇧 English", "set_lang:en")
	btnRU := tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "set_lang:ru")

	inlineKb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(btnUZ),
		tgbotapi.NewInlineKeyboardRow(btnEN),
		tgbotapi.NewInlineKeyboardRow(btnRU),
	)

	reply := tgbotapi.NewMessage(chatID, text)
	reply.ParseMode = tgbotapi.ModeHTML
	reply.ReplyMarkup = inlineKb
	_, _ = bot.Send(reply)
}

func handleMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	if msg == nil || msg.Chat == nil {
		return
	}

	chatID := msg.Chat.ID

	// 1. IMPORTANT: If message is NOT private (it's in a channel, supergroup, or group):
	if !msg.Chat.IsPrivate() {
		// Only check if it's a channel/group to auto-link it, DO NOT run user dialogue!
		if msg.Chat.IsChannel() || msg.Chat.IsSuperGroup() || msg.Chat.IsGroup() {
			detectChannel(bot, msg.Chat)
		}
		return
	}

	userID := msg.From.ID
	username := msg.From.UserName
	text := strings.TrimSpace(msg.Text)

	// Auto-detect channel if message is forwarded from a channel
	if msg.ForwardFromChat != nil {
		chID := fmt.Sprintf("%d", msg.ForwardFromChat.ID)
		setChannelID(chID)
		log.Printf("📢 [FORWARD ORQALI KANAL ULANDI] Nomi: %s, ID: %s", msg.ForwardFromChat.Title, chID)

		resp := fmt.Sprintf(
			"✅ <b>Taklif kanali muvaffaqiyatli ulandi!</b> 🚀\n\n"+
				"📢 <b>Kanal:</b> %s\n"+
				"🆔 <b>Kanal ID:</b> <code>%s</code>\n\n"+
				"Endi barcha buyurtmalar avtomatik shu kanalga tashlanadi!\n\n"+
				"💡 <b>Eslatma:</b> Render.com da <b>Environment</b> bo'limiga kirib <code>TELEGRAM_CHANNEL_ID</code> = <code>%s</code> deb qo'shib qo'ysangiz, server qayta yuklanganda ham kanal doimiy saqlanadi.",
			html.EscapeString(msg.ForwardFromChat.Title),
			chID,
			chID,
		)
		reply := tgbotapi.NewMessage(chatID, resp)
		reply.ParseMode = tgbotapi.ModeHTML
		_, _ = bot.Send(reply)
		return
	}

	// Auto-detect Admin (Haydarali)
	if strings.EqualFold(username, strings.TrimPrefix(developerTG, "@")) || adminChatID == 0 {
		setAdminChatID(chatID)
	}

	state := getOrCreateState(userID, username)

	// Global commands
	switch text {
	case "/start":
		clearState(userID)
		showLanguageSelection(bot, chatID)
		return
	case "/lang", "/language":
		showLanguageSelection(bot, chatID)
		return
	case "/channel", "/kanal":
		cfgMu.RLock()
		curCh := channelID
		cfgMu.RUnlock()
		if curCh != "" {
			resp := fmt.Sprintf("✅ <b>Hozirda ulangan kanal ID:</b> <code>%s</code>\n\nBarcha yangi buyurtmalar ushbu kanalga yuborilmoqda.", curCh)
			reply := tgbotapi.NewMessage(chatID, resp)
			reply.ParseMode = tgbotapi.ModeHTML
			_, _ = bot.Send(reply)
		} else {
			resp := "⚠️ <b>Hozircha hech qanday kanal ulanmagan!</b>\n\n" +
				"Kanalni ulash uchun quyidagilardan birini qiling:\n" +
				"1. Botingizni kanalingizga <b>Admin</b> qiling va <b>'Post Messages'</b> ruxsatini bering.\n" +
				"2. Kanalingizga kirib bitta so'z yozing (masalan: <code>salom</code>) yoki kanaldan istalgan xabarni shu botga <b>Forward</b> qiling!\n\n" +
				"Yoki to'g'ridan-to'g'ri: <code>/setchannel -100xxxxxxxx</code> deb yozing."
			reply := tgbotapi.NewMessage(chatID, resp)
			reply.ParseMode = tgbotapi.ModeHTML
			_, _ = bot.Send(reply)
		}
		return
	case "/order":
		startOrder(bot, chatID, userID, username, state.Language)
		return
	case "/cancel", "❌ Bekor qilish", "❌ Cancel", "❌ Отмена":
		clearState(userID)
		cancelMsg := "❌ <b>Buyurtma bekor qilindi.</b>\nQayta boshlash uchun /order yoki /start bosing."
		if state.Language == LangEN {
			cancelMsg = "❌ <b>Order cancelled.</b>\nPress /order or /start to start again."
		} else if state.Language == LangRU {
			cancelMsg = "❌ <b>Заказ отменен.</b>\nНажмите /order или /start, чтобы начать заново."
		}
		reply := tgbotapi.NewMessage(chatID, cancelMsg)
		reply.ParseMode = tgbotapi.ModeHTML
		reply.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
		_, _ = bot.Send(reply)
		return
	case "/help":
		sendHelp(bot, chatID, state.Language)
		return
	case "/portfolio":
		sendPortfolio(bot, chatID, state.Language)
		return
	}

	if strings.HasPrefix(text, "/setchannel") {
		parts := strings.Fields(text)
		if len(parts) >= 2 {
			setChannelID(parts[1])
			reply := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Kanal ID saqlandi: <code>%s</code>", parts[1]))
			reply.ParseMode = tgbotapi.ModeHTML
			_, _ = bot.Send(reply)
			return
		}
	}

	// FSM State transitions
	switch state.Step {
	case StepChooseLang:
		showLanguageSelection(bot, chatID)

	case StepWaitingName:
		if text == "" {
			promptMsg := "Iltimos, ismingiz va familiyangizni matn ko'rinishida yozing:"
			if state.Language == LangEN {
				promptMsg = "Please enter your full name as text:"
			} else if state.Language == LangRU {
				promptMsg = "Пожалуйста, введите ваше имя и фамилию текстом:"
			}
			reply := tgbotapi.NewMessage(chatID, promptMsg)
			_, _ = bot.Send(reply)
			return
		}
		state.Name = text
		state.Step = StepWaitingContact

		askPhone(bot, chatID, state)

	case StepWaitingContact:
		phone := ""
		if msg.Contact != nil {
			phone = msg.Contact.PhoneNumber
		} else if text != "" {
			phone = text
		}

		if phone == "" {
			errMsg := "Iltimos, telefon raqamingizni kiriting yoki 'Telefon raqamni ulashish' tugmasini bosing:"
			if state.Language == LangEN {
				errMsg = "Please enter your phone number or tap 'Share Phone Number':"
			} else if state.Language == LangRU {
				errMsg = "Пожалуйста, введите номер телефона или нажмите 'Поделиться номером':"
			}
			reply := tgbotapi.NewMessage(chatID, errMsg)
			_, _ = bot.Send(reply)
			return
		}

		state.Phone = phone
		state.Step = StepWaitingProject

		askProjectDetails(bot, chatID, state.Language)

	case StepWaitingProject:
		if text == "" {
			errMsg := "Iltimos, loyihangiz haqida batafsil matn yozing:"
			if state.Language == LangEN {
				errMsg = "Please write detailed information about your project:"
			} else if state.Language == LangRU {
				errMsg = "Пожалуйста, напишите подробную информацию о вашем проекте:"
			}
			reply := tgbotapi.NewMessage(chatID, errMsg)
			_, _ = bot.Send(reply)
			return
		}
		state.ProjectInfo = text
		state.Step = StepWaitingTech

		showTechOptions(bot, chatID, state.Language)

	case StepWaitingTech:
		r, n, v := getTechLabels(state.Language)
		switch {
		case text == r || strings.EqualFold(text, "react") || strings.EqualFold(text, "react.js"):
			finishOrder(bot, chatID, userID, state, r)
		case text == n || strings.EqualFold(text, "next") || strings.EqualFold(text, "next.js"):
			finishOrder(bot, chatID, userID, state, n)
		case text == v || strings.EqualFold(text, "vue") || strings.EqualFold(text, "vue.js"):
			finishOrder(bot, chatID, userID, state, v)
		default:
			showTechOptions(bot, chatID, state.Language)
		}

	default:
		welcomeMsg := "Assalomu alaykum! Yangi loyiha bo'yicha ariza qoldirish uchun /order yoki /start bosing."
		if state.Language == LangEN {
			welcomeMsg = "Hello! To submit a new project order, press /order or /start."
		} else if state.Language == LangRU {
			welcomeMsg = "Здравствуйте! Чтобы оставить заявку на проект, нажмите /order или /start."
		}
		reply := tgbotapi.NewMessage(chatID, welcomeMsg)
		_, _ = bot.Send(reply)
	}
}

func askPhone(bot *tgbotapi.BotAPI, chatID int64, state *UserState) {
	var promptText, btnContactText, btnCancelText string

	switch state.Language {
	case LangEN:
		promptText = fmt.Sprintf(
			"Thank you, <b>%s</b>!\n\n"+
				"2️⃣ <b>Please enter your phone number:</b>\n"+
				"<i>(You can tap '📱 Share Phone Number' below or type it manually)</i>",
			html.EscapeString(state.Name),
		)
		btnContactText = "📱 Share Phone Number"
		btnCancelText = "❌ Cancel"
	case LangRU:
		promptText = fmt.Sprintf(
			"Спасибо, <b>%s</b>!\n\n"+
				"2️⃣ <b>Введите ваш номер телефона для связи:</b>\n"+
				"<i>(Вы можете нажать кнопку '📱 Поделиться номером' ниже или написать вручную)</i>",
			html.EscapeString(state.Name),
		)
		btnContactText = "📱 Поделиться номером"
		btnCancelText = "❌ Отмена"
	default:
		promptText = fmt.Sprintf(
			"Rahmat, <b>%s</b>!\n\n"+
				"2️⃣ <b>Bog'lanish uchun telefon raqamingizni kiriting:</b>\n"+
				"<i>(Pastdagi '📱 Telefon raqamni ulashish' tugmasini bosishingiz yoki raqamingizni yozib yuborishingiz mumkin)</i>",
			html.EscapeString(state.Name),
		)
		btnContactText = "📱 Telefon raqamni ulashish"
		btnCancelText = "❌ Bekor qilish"
	}

	reply := tgbotapi.NewMessage(chatID, promptText)
	reply.ParseMode = tgbotapi.ModeHTML

	contactBtn := tgbotapi.NewKeyboardButtonContact(btnContactText)
	cancelBtn := tgbotapi.NewKeyboardButton(btnCancelText)
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(contactBtn),
		tgbotapi.NewKeyboardButtonRow(cancelBtn),
	)
	kb.ResizeKeyboard = true
	kb.OneTimeKeyboard = true
	reply.ReplyMarkup = kb
	_, _ = bot.Send(reply)
}

func askProjectDetails(bot *tgbotapi.BotAPI, chatID int64, l Lang) {
	var prompt string
	switch l {
	case LangEN:
		prompt = "Great! Now please describe your project in detail. 🎯\n\n" +
			"3️⃣ <b>What kind of project do you need? Provide full details:</b>\n\n" +
			"💡 <b>Recommended information:</b>\n" +
			"• Project type (Landing page, Corporate website, E-commerce, CRM, Web application)\n" +
			"• Key features and required sections\n" +
			"• Estimated deadline and budget\n" +
			"• Any additional preferences or references\n\n" +
			"Please write everything in detail:"
	case LangRU:
		prompt = "Отлично! Теперь расскажите подробно о вашем проекте. 🎯\n\n" +
			"3️⃣ <b>Какой проект вам нужен? Предоставьте полную информацию:</b>\n\n" +
			"💡 <b>Рекомендуемые данные:</b>\n" +
			"• Тип проекта (Landing page, Корпоративный сайт, Интернет-магазин, CRM, Веб-приложение)\n" +
			"• Необходимые ключевые функции и страницы\n" +
			"• Примерные сроки и бюджет\n" +
			"• Любые дополнительные пожелания\n\n" +
			"Пожалуйста, опишите всё подробно:"
	default:
		prompt = "Ajoyib! Endi esa loyiha haqida to'liq informatsiya bering. 🎯\n\n" +
			"3️⃣ <b>Qanaqa loyiha qilmoqchisiz? Loyiha haqida to'liq ma'lumot bering:</b>\n\n" +
			"💡 <b>Tavsiya qilinadigan ma'lumotlar:</b>\n" +
			"• Loyiha turi (Landing page, Korporativ sayt, Online do'kon, CRM, Web dastur)\n" +
			"• Kerakli asosiy funksiyalar va imkoniyatlar\n" +
			"• Taxminiy topshirish muddati va byudjet\n" +
			"• Boshqa qo'shimcha talablar\n\n" +
			"Iltimos, barchasini batafsil yozib qoldiring:"
	}

	reply := tgbotapi.NewMessage(chatID, prompt)
	reply.ParseMode = tgbotapi.ModeHTML
	reply.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	_, _ = bot.Send(reply)
}

func showTechOptions(bot *tgbotapi.BotAPI, chatID int64, l Lang) {
	r, n, v := getTechLabels(l)

	var prompt string
	switch l {
	case LangEN:
		prompt = "4️⃣ <b>Which technology do you prefer for your project?</b>\n\n" +
			"Please select one of the 3 options below:\n\n" +
			"• <b>" + r + "</b>\n" +
			"• <b>" + n + "</b>\n" +
			"• <b>" + v + "</b>"
	case LangRU:
		prompt = "4️⃣ <b>Какую технологию вы предпочитаете для вашего проекта?</b>\n\n" +
			"Пожалуйста, выберите один из 3 вариантов ниже:\n\n" +
			"• <b>" + r + "</b>\n" +
			"• <b>" + n + "</b>\n" +
			"• <b>" + v + "</b>"
	default:
		prompt = "4️⃣ <b>Loyihangiz uchun qaysi texnologiyani ma'qul ko'rasiz?</b>\n\n" +
			"Quyidagi 3 ta variantdan birini tanlang:\n\n" +
			"• <b>" + r + "</b>\n" +
			"• <b>" + n + "</b>\n" +
			"• <b>" + v + "</b>"
	}

	btn1 := tgbotapi.NewInlineKeyboardButtonData(r, "tech:react")
	btn2 := tgbotapi.NewInlineKeyboardButtonData(n, "tech:next")
	btn3 := tgbotapi.NewInlineKeyboardButtonData(v, "tech:vue")

	inlineKb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(btn1),
		tgbotapi.NewInlineKeyboardRow(btn2),
		tgbotapi.NewInlineKeyboardRow(btn3),
	)

	reply := tgbotapi.NewMessage(chatID, prompt)
	reply.ParseMode = tgbotapi.ModeHTML
	reply.ReplyMarkup = inlineKb
	_, _ = bot.Send(reply)
}

func handleCallback(bot *tgbotapi.BotAPI, cb *tgbotapi.CallbackQuery) {
	if cb == nil || cb.From == nil {
		return
	}
	userID := cb.From.ID
	chatID := int64(0)
	if cb.Message != nil {
		chatID = cb.Message.Chat.ID
	}

	// Answer callback to remove spinner
	_, _ = bot.Request(tgbotapi.NewCallback(cb.ID, ""))

	state := getOrCreateState(userID, cb.From.UserName)

	// Language selection callback
	if strings.HasPrefix(cb.Data, "set_lang:") {
		langCode := strings.TrimPrefix(cb.Data, "set_lang:")
		switch langCode {
		case "en":
			state.Language = LangEN
		case "ru":
			state.Language = LangRU
		default:
			state.Language = LangUZ
		}
		state.Step = StepWaitingName

		startFlow(bot, chatID, userID, cb.From.UserName, cb.From.FirstName, state.Language)
		return
	}

	// Technology selection callback
	r, n, v := getTechLabels(state.Language)
	var selectedTech string
	switch cb.Data {
	case "tech:react":
		selectedTech = r
	case "tech:next":
		selectedTech = n
	case "tech:vue":
		selectedTech = v
	default:
		return
	}

	if state.Name == "" || state.ProjectInfo == "" {
		startOrder(bot, chatID, userID, cb.From.UserName, state.Language)
		return
	}

	finishOrder(bot, chatID, userID, state, selectedTech)
}

func finishOrder(bot *tgbotapi.BotAPI, chatID int64, userID int64, state *UserState, selectedTech string) {
	state.Technology = selectedTech
	state.Step = StepChooseLang

	nowStr := time.Now().Format("2006-01-02 15:04:05")

	tgDisplay := "ko'rsatilmagan"
	if state.Telegram != "" {
		tgDisplay = "@" + state.Telegram
	}

	// 1. Reply to client in their chosen language
	var clientSummary string
	switch state.Language {
	case LangEN:
		clientSummary = fmt.Sprintf(
			"🎉 <b>Thank you! Your project order has been successfully received!</b>\n\n"+
				"📋 <b>Order Summary:</b>\n"+
				"👤 <b>Client:</b> %s\n"+
				"📞 <b>Phone:</b> %s\n"+
				"✈️ <b>Telegram:</b> %s\n"+
				"🛠 <b>Selected Technology:</b> <b>%s</b>\n\n"+
				"📝 <b>Project Details:</b>\n"+
				"<i>%s</i>\n\n"+
				"━━━━━━━━━━━━━━━━━━━━━\n"+
				"⏳ <b>Haydarali Akbarov</b> will contact you shortly!\n\n"+
				"Direct Contact: @haydaraliakbarov | +998 88 083 19 88",
			html.EscapeString(state.Name),
			html.EscapeString(state.Phone),
			html.EscapeString(tgDisplay),
			html.EscapeString(state.Technology),
			html.EscapeString(state.ProjectInfo),
		)
	case LangRU:
		clientSummary = fmt.Sprintf(
			"🎉 <b>Спасибо! Ваш заказ успешно принят!</b>\n\n"+
				"📋 <b>Детали заказа:</b>\n"+
				"👤 <b>Клиент:</b> %s\n"+
				"📞 <b>Телефон:</b> %s\n"+
				"✈️ <b>Telegram:</b> %s\n"+
				"🛠 <b>Выбранная технология:</b> <b>%s</b>\n\n"+
				"📝 <b>О проекте:</b>\n"+
				"<i>%s</i>\n\n"+
				"━━━━━━━━━━━━━━━━━━━━━\n"+
				"⏳ <b>Хайдарали Акбаров</b> свяжется с вами в ближайшее время!\n\n"+
				"Контакты: @haydaraliakbarov | +998 88 083 19 88",
			html.EscapeString(state.Name),
			html.EscapeString(state.Phone),
			html.EscapeString(tgDisplay),
			html.EscapeString(state.Technology),
			html.EscapeString(state.ProjectInfo),
		)
	default:
		clientSummary = fmt.Sprintf(
			"🎉 <b>Rahmat! Buyurtmangiz muvaffaqiyatli qabul qilindi!</b>\n\n"+
				"📋 <b>Buyurtma xulosasi:</b>\n"+
				"👤 <b>Mijoz:</b> %s\n"+
				"📞 <b>Telefon:</b> %s\n"+
				"✈️ <b>Telegram:</b> %s\n"+
				"🛠 <b>Tanlangan texnologiya:</b> <b>%s</b>\n\n"+
				"📝 <b>Loyiha haqida to'liq ma'lumot:</b>\n"+
				"<i>%s</i>\n\n"+
				"━━━━━━━━━━━━━━━━━━━━━\n"+
				"⏳ Tez orada <b>Haydarali Akbarov</b> siz bilan bog'lanadi!\n\n"+
				"Aloqa: @haydaraliakbarov | +998 88 083 19 88",
			html.EscapeString(state.Name),
			html.EscapeString(state.Phone),
			html.EscapeString(tgDisplay),
			html.EscapeString(state.Technology),
			html.EscapeString(state.ProjectInfo),
		)
	}

	reply := tgbotapi.NewMessage(chatID, clientSummary)
	reply.ParseMode = tgbotapi.ModeHTML
	_, _ = bot.Send(reply)

	// 2. ROUTING: KANALGA VA SIZGA (MEN) YUBORISH
	sendOrderRouting(bot, state, userID, chatID, nowStr, "Telegram Bot")

	// 3. Clear session
	clearState(userID)
}

func sendOrderRouting(bot *tgbotapi.BotAPI, state *UserState, userID int64, chatID int64, timestamp, source string) {
	tgUserLink := "Mavjud emas"
	if state.Telegram != "" {
		tgUserLink = fmt.Sprintf("@%s (ID: <code>%d</code>)", state.Telegram, userID)
	} else if userID > 0 {
		tgUserLink = fmt.Sprintf("<a href=\"tg://user?id=%d\">Foydalanuvchi</a> (ID: <code>%d</code>)", userID, userID)
	}

	orderText := fmt.Sprintf(
		"📢 <b>YANGI LOYIHA BUYURTMASI!</b> 🚀\n"+
			"━━━━━━━━━━━━━━━━━━━━━━━━━\n"+
			"👤 <b>Mijoz:</b> %s\n"+
			"📞 <b>Telefon / Aloqa:</b> <code>%s</code>\n"+
			"✈️ <b>Telegram:</b> %s\n"+
			"🌐 <b>Tanlangan til:</b> %s\n"+
			"🛠 <b>Tanlangan texnologiya:</b> <b>%s</b>\n"+
			"━━━━━━━━━━━━━━━━━━━━━━━━━\n"+
			"📋 <b>Loyiha haqida to'liq ma'lumot:</b>\n"+
			"%s\n"+
			"━━━━━━━━━━━━━━━━━━━━━━━━━\n"+
			"📅 <b>Vaqt:</b> %s\n"+
			"🌐 <b>Manba:</b> %s",
		html.EscapeString(state.Name),
		html.EscapeString(state.Phone),
		tgUserLink,
		getLangName(state.Language),
		html.EscapeString(state.Technology),
		html.EscapeString(state.ProjectInfo),
		html.EscapeString(timestamp),
		html.EscapeString(source),
	)

	cfgMu.RLock()
	curChan := channelID
	curAdmin := adminChatID
	cfgMu.RUnlock()

	// 1. Send to Channel (bot -> kanal)
	if curChan != "" {
		var err error
		if strings.HasPrefix(curChan, "-") {
			idNum, parseErr := strconv.ParseInt(curChan, 10, 64)
			if parseErr == nil {
				msg := tgbotapi.NewMessage(idNum, orderText)
				msg.ParseMode = tgbotapi.ModeHTML
				_, err = bot.Send(msg)
			} else {
				err = parseErr
			}
		} else {
			chanName := curChan
			if !strings.HasPrefix(chanName, "@") {
				chanName = "@" + chanName
			}
			msg := tgbotapi.NewMessageToChannel(chanName, orderText)
			msg.ParseMode = tgbotapi.ModeHTML
			_, err = bot.Send(msg)
		}

		if err != nil {
			log.Printf("❌ Kanalga yuborishda xatolik (%s): %v", curChan, err)
			if chatID != 0 {
				warn := tgbotapi.NewMessage(chatID, fmt.Sprintf("⚠️ <b>Kanalga yuborishda xatolik:</b> %v\n\nIltimos, bot kanalingizda <b>Admin</b> qilinganiga va <b>'Post Messages'</b> huquqi borligiga ishonch hosil qiling!", err))
				warn.ParseMode = tgbotapi.ModeHTML
				_, _ = bot.Send(warn)
			}
		} else {
			log.Printf("✅ Buyurtma kanalga yuborildi: %s", curChan)
		}
	} else {
		log.Println("⚠️ Kanal ID hali ulanmagan. Kanaldan xabar forward qiling.")
		if chatID != 0 {
			warnMsg := "⚠️ <b>Eslatma:</b> Buyurtmangiz qabul qilindi, lekin Telegram kanali hali ulanmagan!\n\n" +
				"Kanalni ulash uchun: Kanalingizga kirib bitta so'z yozing (masalan <code>salom</code>) yoki o'sha kanaldan bitta xabarni shu botga <b>Forward</b> qiling!"
			warn := tgbotapi.NewMessage(chatID, warnMsg)
			warn.ParseMode = tgbotapi.ModeHTML
			_, _ = bot.Send(warn)
		}
	}

	// 2. Send to Admin personally (kanal -> men)
	if curAdmin != 0 {
		adminMsg := tgbotapi.NewMessage(curAdmin, "🔔 <b>Yangi buyurtma (Kanalga ham yuborildi):</b>\n\n"+orderText)
		adminMsg.ParseMode = tgbotapi.ModeHTML
		_, err := bot.Send(adminMsg)
		if err != nil {
			log.Printf("Warning: failed to notify admin: %v", err)
		} else {
			log.Printf("✅ Buyurtma shaxsan sizga ham yetkazildi (Admin: %d)", curAdmin)
		}
	}
}

func startFlow(bot *tgbotapi.BotAPI, chatID int64, userID int64, username, firstName string, l Lang) {
	state := getOrCreateState(userID, username)
	state.Step = StepWaitingName
	state.Language = l

	var msgText string
	switch l {
	case LangEN:
		msgText = fmt.Sprintf(
			"1️⃣ <b>Please enter your full name (or company name):</b>",
		)
	case LangRU:
		msgText = fmt.Sprintf(
			"1️⃣ <b>Пожалуйста, введите ваше имя и фамилию (или название компании):</b>",
		)
	default:
		msgText = fmt.Sprintf(
			"1️⃣ <b>Iltimos, ismingiz va familiyangizni (yoki kompaniyangiz nomini) kiriting:</b>",
		)
	}

	reply := tgbotapi.NewMessage(chatID, msgText)
	reply.ParseMode = tgbotapi.ModeHTML
	reply.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	_, _ = bot.Send(reply)
}

func startOrder(bot *tgbotapi.BotAPI, chatID int64, userID int64, username string, l Lang) {
	state := getOrCreateState(userID, username)
	state.Step = StepWaitingName
	state.Language = l

	var msgText string
	switch l {
	case LangEN:
		msgText = "🚀 <b>Submit a New Project Order</b>\n\n" +
			"1️⃣ <b>Please enter your full name (or company name):</b>"
	case LangRU:
		msgText = "🚀 <b>Оформить заказ на новый проект</b>\n\n" +
			"1️⃣ <b>Пожалуйста, введите ваше имя и фамилию (или название компании):</b>"
	default:
		msgText = "🚀 <b>Yangi loyiha bo'yicha buyurtma berish</b>\n\n" +
			"1️⃣ <b>Iltimos, ismingiz va familiyangizni (yoki kompaniyangiz nomini) kiriting:</b>"
	}

	reply := tgbotapi.NewMessage(chatID, msgText)
	reply.ParseMode = tgbotapi.ModeHTML
	reply.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	_, _ = bot.Send(reply)
}

func sendHelp(bot *tgbotapi.BotAPI, chatID int64, l Lang) {
	var helpText string
	switch l {
	case LangEN:
		helpText = "ℹ️ <b>Haydarali Akbarov — Portfolio Bot</b>\n\n" +
			"📌 <b>Commands:</b>\n" +
			"/start — Start bot and choose language\n" +
			"/lang — Change language\n" +
			"/order — Submit a new project request\n" +
			"/channel — Check channel link status\n" +
			"/portfolio — View portfolio and skills\n" +
			"/cancel — Cancel current process\n\n" +
			"📞 <b>Contact Developer:</b>\n" +
			"Telegram: @haydaraliakbarov\n" +
			"Phone: +998 88 083 19 88"
	case LangRU:
		helpText = "ℹ️ <b>Хайдарали Акбаров — Портфолио-бот</b>\n\n" +
			"📌 <b>Команды:</b>\n" +
			"/start — Запустить бота и выбрать язык\n" +
			"/lang — Сменить язык\n" +
			"/order — Оформить заказ на проект\n" +
			"/channel — Проверить статус привязки канала\n" +
			"/portfolio — Посмотреть портфолио разработчика\n" +
			"/cancel — Отменить текущее действие\n\n" +
			"📞 <b>Контакты разработчика:</b>\n" +
			"Telegram: @haydaraliakbarov\n" +
			"Телефон: +998 88 083 19 88"
	default:
		helpText = "ℹ️ <b>Haydarali Akbarov — Portfolio Bot</b>\n\n" +
			"📌 <b>Buyruqlar:</b>\n" +
			"/start — Botni ishga tushirish va tilni tanlash\n" +
			"/lang — Tilni o'zgartirish\n" +
			"/order — Yangi loyiha buyurtma berish\n" +
			"/channel — Kanal ulanish holatini tekshirish\n" +
			"/portfolio — Dasturchi portfoliosi\n" +
			"/cancel — Jarayonni bekor qilish\n\n" +
			"📞 <b>Aloqa:</b>\n" +
			"Telegram: @haydaraliakbarov\n" +
			"Telefon: +998 88 083 19 88"
	}

	reply := tgbotapi.NewMessage(chatID, helpText)
	reply.ParseMode = tgbotapi.ModeHTML
	_, _ = bot.Send(reply)
}

func sendPortfolio(bot *tgbotapi.BotAPI, chatID int64, l Lang) {
	r, n, v := getTechLabels(l)

	var portfolioText string
	switch l {
	case LangEN:
		portfolioText = "🌐 <b>Akbarov Haydarali — Frontend Web Developer</b>\n\n" +
			"Specialist in building modern, performant, and responsive web applications.\n\n" +
			"🛠 <b>Main Stack:</b>\n" +
			"• " + r + "\n" +
			"• " + n + "\n" +
			"• " + v + "\n\n" +
			"🔗 <b>GitHub:</b> github.com/akbarovhaydarali886-star\n\n" +
			"Press /order to submit a project!"
	case LangRU:
		portfolioText = "🌐 <b>Хайдарали Акбаров — Frontend веб-разработчик</b>\n\n" +
			"Специалист по созданию современных, быстрых и удобных веб-приложений.\n\n" +
			"🛠 <b>Основной стек:</b>\n" +
			"• " + r + "\n" +
			"• " + n + "\n" +
			"• " + v + "\n\n" +
			"🔗 <b>GitHub:</b> github.com/akbarovhaydarali886-star\n\n" +
			"Нажмите /order, чтобы заказать проект!"
	default:
		portfolioText = "🌐 <b>Akbarov Haydarali — Frontend Web Dasturchi</b>\n\n" +
			"Zamonaviy va sifatli veb-saytlar hamda web-ilovalarni yaratish bo'yicha mutaxassis.\n\n" +
			"🛠 <b>Texnologiyalar:</b>\n" +
			"• " + r + "\n" +
			"• " + n + "\n" +
			"• " + v + "\n\n" +
			"🔗 <b>GitHub:</b> github.com/akbarovhaydarali886-star\n\n" +
			"Buyurtma berish uchun /order bosing!"
	}

	reply := tgbotapi.NewMessage(chatID, portfolioText)
	reply.ParseMode = tgbotapi.ModeHTML
	_, _ = bot.Send(reply)
}

func handleWebOrder(bot *tgbotapi.BotAPI, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req struct {
		Name       string `json:"name"`
		Phone      string `json:"phone"`
		Telegram   string `json:"telegram"`
		Project    string `json:"project"`
		Technology string `json:"technology"`
		Language   string `json:"language"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "JSON error"})
		return
	}

	l := LangUZ
	if req.Language == "en" {
		l = LangEN
	} else if req.Language == "ru" {
		l = LangRU
	}

	rTech, nTech, vTech := getTechLabels(l)
	tech := req.Technology
	switch strings.ToLower(tech) {
	case "react", "react.js":
		tech = rTech
	case "next", "next.js":
		tech = nTech
	case "vue", "vue.js":
		tech = vTech
	default:
		tech = "Ko'rsatilmagan"
	}

	state := &UserState{
		Language:    l,
		Name:        req.Name,
		Phone:       req.Phone,
		Telegram:    req.Telegram,
		ProjectInfo: req.Project,
		Technology:  tech,
		UpdatedAt:   time.Now(),
	}

	nowStr := time.Now().Format("2006-01-02 15:04:05")
	sendOrderRouting(bot, state, 0, 0, nowStr, "Portfolio Web-sayti")

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Qabul qilindi"})
}

// Helpers
func getOrCreateState(userID int64, username string) *UserState {
	statesMu.Lock()
	defer statesMu.Unlock()

	s, ok := userStates[userID]
	if !ok {
		s = &UserState{
			Language:  LangUZ,
			Step:      StepChooseLang,
			Telegram:  username,
			UserID:    userID,
			UpdatedAt: time.Now(),
		}
		userStates[userID] = s
	} else if username != "" {
		s.Telegram = username
	}
	s.UpdatedAt = time.Now()
	return s
}

func clearState(userID int64) {
	statesMu.Lock()
	defer statesMu.Unlock()
	delete(userStates, userID)
}

func setChannelID(newChan string) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	channelID = newChan
	_ = os.Setenv("TELEGRAM_CHANNEL_ID", newChan)
	updateEnv("TELEGRAM_CHANNEL_ID", newChan)
}

func setAdminChatID(newAdmin int64) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	adminChatID = newAdmin
	_ = os.Setenv("TELEGRAM_ADMIN_CHAT_ID", fmt.Sprintf("%d", newAdmin))
	updateEnv("TELEGRAM_ADMIN_CHAT_ID", fmt.Sprintf("%d", newAdmin))
}

func loadConfig() {
	loadDotEnv(".env")
	loadDotEnv("../.env")

	botToken = getEnv("TELEGRAM_BOT_TOKEN", "8851269459:AAEpHeyEg2hgg0_TqfjYYbuyb0pGTPRJAG8")
	channelID = getEnv("TELEGRAM_CHANNEL_ID", "")
	adminChatID, _ = strconv.ParseInt(getEnv("TELEGRAM_ADMIN_CHAT_ID", "0"), 10, 64)
	port = getEnv("PORT", "8080")
	portfolioURL = getEnv("PORTFOLIO_URL", "https://haydarali.uz")
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return def
}

func updateEnv(key, value string) {
	filepath := ".env"
	content, err := os.ReadFile(filepath)
	if err != nil {
		filepath = "../.env"
		content, err = os.ReadFile(filepath)
		if err != nil {
			return
		}
	}
	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), key+"=") {
			lines[i] = fmt.Sprintf("%s=%s", key, value)
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}
	_ = os.WriteFile(filepath, []byte(strings.Join(lines, "\n")), 0644)
}

func loadDotEnv(filepath string) {
	f, err := os.Open(filepath)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			if (strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"")) ||
				(strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'")) {
				v = v[1 : len(v)-1]
			}
			if _, exists := os.LookupEnv(k); !exists {
				_ = os.Setenv(k, v)
			}
		}
	}
}
