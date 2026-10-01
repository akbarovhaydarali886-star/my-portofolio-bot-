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

// FSM Steps
const (
	StepStart = iota
	StepWaitingName
	StepWaitingContact
	StepWaitingProject
	StepWaitingTech
)

// Technologies with icons
const (
	TechReact = "⚛️ React.js (Qiyin ishlar uchun)"
	TechNext  = "▲ Next.js (Oson ishlar uchun)"
	TechVue   = "🟢 Vue.js (Oson ishlar uchun)"
)

// UserState stores client state in RAM
type UserState struct {
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
		tgbotapi.BotCommand{Command: "start", Description: "Botni ishga tushirish / Yangi buyurtma"},
		tgbotapi.BotCommand{Command: "order", Description: "Loyiha bo'yicha ariza qoldirish"},
		tgbotapi.BotCommand{Command: "portfolio", Description: "Haydarali Akbarov portfoliosi"},
		tgbotapi.BotCommand{Command: "help", Description: "Yordam va aloqa"},
		tgbotapi.BotCommand{Command: "cancel", Description: "Bekor qilish"},
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
				"service":     "portfolio-bot-webhook",
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
	} else {
		log.Println("⚡️ RENDER_EXTERNAL_URL topilmadi. Lokal Long-Polling rejimida ishlamoqda...")
		// Delete any previous webhook so polling works
		_, _ = bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: false})

		u := tgbotapi.NewUpdate(0)
		u.Timeout = 30
		updates = bot.GetUpdatesChan(u)

		go func() {
			log.Printf("🚀 Lokal healthcheck server 0.0.0.0:%s da ishlamoqda...", port)
			_ = http.ListenAndServe("0.0.0.0:"+port, nil)
		}()
	}

	log.Println("🤖 Bot xabarlarni qabul qilishga to'liq tayyor!")

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
	log.Printf("📢 [KANAL ANIQLANDI] Nomi: '%s', Username: '%s', ID: %s", chat.Title, chat.UserName, chID)

	setChannelID(chID)

	confirmText := fmt.Sprintf(
		"✅ <b>Haydarali Akbarov Portfolio Boti ulandi!</b> 🚀\n\n"+
			"Ushbu kanal (<b>%s</b>) yangi loyiha takliflari va buyurtmalarni qabul qilish uchun muvaffaqiyatli sozlandi.",
		html.EscapeString(chat.Title),
	)
	msg := tgbotapi.NewMessage(chat.ID, confirmText)
	msg.ParseMode = tgbotapi.ModeHTML
	_, _ = bot.Send(msg)
}

func handleMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	userID := msg.From.ID
	username := msg.From.UserName
	text := strings.TrimSpace(msg.Text)

	// Auto-detect channel if message is forwarded from a channel
	if msg.ForwardFromChat != nil && msg.ForwardFromChat.IsChannel() {
		chID := fmt.Sprintf("%d", msg.ForwardFromChat.ID)
		setChannelID(chID)
		log.Printf("📢 [FORWARD ORQALI KANAL ULANDI] Nomi: %s, ID: %s", msg.ForwardFromChat.Title, chID)

		resp := fmt.Sprintf(
			"✅ <b>Taklif kanali muvaffaqiyatli ulandi!</b>\n\n"+
				"📢 <b>Kanal:</b> %s\n"+
				"🆔 <b>ID:</b> <code>%s</code>\n\n"+
				"Endi barcha buyurtmalar avtomatik shu kanalga tashlanadi!",
			html.EscapeString(msg.ForwardFromChat.Title),
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

	// Commands
	switch text {
	case "/start":
		startFlow(bot, chatID, userID, username, msg.From.FirstName)
		return
	case "/order":
		startOrder(bot, chatID, userID, username)
		return
	case "/cancel", "❌ Bekor qilish":
		clearState(userID)
		reply := tgbotapi.NewMessage(chatID, "❌ <b>Buyurtma bekor qilindi.</b>\n\nQayta boshlash uchun /order yoki /start bosing.")
		reply.ParseMode = tgbotapi.ModeHTML
		reply.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
		_, _ = bot.Send(reply)
		return
	case "/help":
		helpText := "ℹ️ <b>Haydarali Akbarov — Portfolio Bot</b>\n\n" +
			"📌 <b>Buyruqlar:</b>\n" +
			"/start — Botni ishga tushirish\n" +
			"/order — Yangi loyiha buyurtma berish\n" +
			"/portfolio — Dasturchi portfoliosi\n" +
			"/cancel — Jarayonni bekor qilish\n\n" +
			"💡 <b>Kanal ulash:</b> Kanaldagi istalgan xabarni shu botga forward qiling yoki <code>/setchannel -100xxxxxxxx</code> yozing.\n\n" +
			"📞 <b>Aloqa:</b>\n" +
			"Telegram: @haydaraliakbarov\n" +
			"Telefon: +998 88 083 19 88"
		reply := tgbotapi.NewMessage(chatID, helpText)
		reply.ParseMode = tgbotapi.ModeHTML
		_, _ = bot.Send(reply)
		return
	case "/portfolio":
		portfolioText := "🌐 <b>Akbarov Haydarali — Frontend Web Dasturchi</b>\n\n" +
			"Zamonaviy va sifatli veb-saytlar hamda web-ilovalarni yaratish bo'yicha mutaxassis.\n\n" +
			"🛠 <b>Texnologiyalar:</b>\n" +
			"• " + TechReact + "\n" +
			"• " + TechNext + "\n" +
			"• " + TechVue + "\n\n" +
			"🔗 <b>GitHub:</b> github.com/akbarovhaydarali886-star\n\n" +
			"Buyurtma berish uchun /order bosing!"
		reply := tgbotapi.NewMessage(chatID, portfolioText)
		reply.ParseMode = tgbotapi.ModeHTML
		_, _ = bot.Send(reply)
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
	state := getOrCreateState(userID, username)

	switch state.Step {
	case StepWaitingName:
		if text == "" {
			reply := tgbotapi.NewMessage(chatID, "Iltimos, ismingiz va familiyangizni matn ko'rinishida yozing:")
			_, _ = bot.Send(reply)
			return
		}
		state.Name = text
		state.Step = StepWaitingContact

		replyText := fmt.Sprintf(
			"Rahmat, <b>%s</b>!\n\n"+
				"2️⃣ <b>Bog'lanish uchun telefon raqamingizni kiriting:</b>\n"+
				"<i>(Pastdagi '📱 Telefon raqamni ulashish' tugmasini bosishingiz yoki raqamingizni yozib yuborishingiz mumkin)</i>",
			html.EscapeString(state.Name),
		)
		reply := tgbotapi.NewMessage(chatID, replyText)
		reply.ParseMode = tgbotapi.ModeHTML

		contactBtn := tgbotapi.NewKeyboardButtonContact("📱 Telefon raqamni ulashish")
		cancelBtn := tgbotapi.NewKeyboardButton("❌ Bekor qilish")
		kb := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(contactBtn),
			tgbotapi.NewKeyboardButtonRow(cancelBtn),
		)
		kb.ResizeKeyboard = true
		kb.OneTimeKeyboard = true
		reply.ReplyMarkup = kb
		_, _ = bot.Send(reply)

	case StepWaitingContact:
		phone := ""
		if msg.Contact != nil {
			phone = msg.Contact.PhoneNumber
		} else if text != "" {
			phone = text
		}

		if phone == "" {
			reply := tgbotapi.NewMessage(chatID, "Iltimos, telefon raqamingizni kiriting yoki 'Telefon raqamni ulashish' tugmasini bosing:")
			_, _ = bot.Send(reply)
			return
		}

		state.Phone = phone
		state.Step = StepWaitingProject

		prompt := "Ajoyib! Endi esa loyiha haqida to'liq informatsiya bering. 🎯\n\n" +
			"3️⃣ <b>Qanaqa loyiha qilmoqchisiz? Loyiha haqida to'liq ma'lumot bering:</b>\n\n" +
			"💡 <b>Tavsiya qilinadigan ma'lumotlar:</b>\n" +
			"• Loyiha turi (Landing page, Korporativ sayt, Online do'kon, CRM, Web dastur)\n" +
			"• Kerakli asosiy funksiyalar va imkoniyatlar\n" +
			"• Taxminiy topshirish muddati va byudjet\n" +
			"• Boshqa qo'shimcha talablar\n\n" +
			"Iltimos, barchasini batafsil yozib qoldiring:"

		reply := tgbotapi.NewMessage(chatID, prompt)
		reply.ParseMode = tgbotapi.ModeHTML
		reply.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
		_, _ = bot.Send(reply)

	case StepWaitingProject:
		if text == "" {
			reply := tgbotapi.NewMessage(chatID, "Iltimos, loyihangiz haqida batafsil matn yozing:")
			_, _ = bot.Send(reply)
			return
		}
		state.ProjectInfo = text
		state.Step = StepWaitingTech

		showTechOptions(bot, chatID)

	case StepWaitingTech:
		switch text {
		case TechReact, "React", "React.js", "React JS":
			finishOrder(bot, chatID, userID, state, TechReact)
		case TechNext, "Next", "Next.js", "Next JS":
			finishOrder(bot, chatID, userID, state, TechNext)
		case TechVue, "Vue", "Vue.js", "Vue JS":
			finishOrder(bot, chatID, userID, state, TechVue)
		default:
			showTechOptions(bot, chatID)
		}

	default:
		reply := tgbotapi.NewMessage(chatID, "Assalomu alaykum! Loyiha bo'yicha ariza qoldirish uchun /order yoki /start bosing.")
		_, _ = bot.Send(reply)
	}
}

func showTechOptions(bot *tgbotapi.BotAPI, chatID int64) {
	prompt := "4️⃣ <b>Loyihangiz uchun qaysi texnologiyani ma'qul ko'rasiz?</b>\n\n" +
		"Quyidagi 3 ta variantdan birini tanlang:\n\n" +
		"• <b>" + TechReact + "</b>\n" +
		"• <b>" + TechNext + "</b>\n" +
		"• <b>" + TechVue + "</b>"

	btn1 := tgbotapi.NewInlineKeyboardButtonData(TechReact, "tech:react")
	btn2 := tgbotapi.NewInlineKeyboardButtonData(TechNext, "tech:next")
	btn3 := tgbotapi.NewInlineKeyboardButtonData(TechVue, "tech:vue")

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

	var selectedTech string
	switch cb.Data {
	case "tech:react":
		selectedTech = TechReact
	case "tech:next":
		selectedTech = TechNext
	case "tech:vue":
		selectedTech = TechVue
	default:
		return
	}

	if state.Name == "" || state.ProjectInfo == "" {
		startOrder(bot, chatID, userID, cb.From.UserName)
		return
	}

	finishOrder(bot, chatID, userID, state, selectedTech)
}

func finishOrder(bot *tgbotapi.BotAPI, chatID int64, userID int64, state *UserState, selectedTech string) {
	state.Technology = selectedTech
	state.Step = StepStart

	nowStr := time.Now().Format("2006-01-02 15:04:05")

	tgDisplay := "ko'rsatilmagan"
	if state.Telegram != "" {
		tgDisplay = "@" + state.Telegram
	}

	// 1. Reply to client
	clientSummary := fmt.Sprintf(
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

	reply := tgbotapi.NewMessage(chatID, clientSummary)
	reply.ParseMode = tgbotapi.ModeHTML
	_, _ = bot.Send(reply)

	// 2. ROUTING: KANALGA VA SIZGA (MEN) YUBORISH
	// botga so'rov -> bot -> kanal -> men
	sendOrderRouting(bot, state, userID, nowStr, "Telegram Bot")

	// 3. Clear session
	clearState(userID)
}

func sendOrderRouting(bot *tgbotapi.BotAPI, state *UserState, userID int64, timestamp, source string) {
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
		if strings.HasPrefix(curChan, "-") {
			// Numeric ID
			idNum, err := strconv.ParseInt(curChan, 10, 64)
			if err == nil {
				msg := tgbotapi.NewMessage(idNum, orderText)
				msg.ParseMode = tgbotapi.ModeHTML
				_, err = bot.Send(msg)
				if err != nil {
					log.Printf("❌ Kanalga yuborishda xatolik (%s): %v", curChan, err)
				} else {
					log.Printf("✅ Buyurtma kanalga yuborildi: %s", curChan)
				}
			}
		} else {
			// Channel username (@channel)
			chanName := curChan
			if !strings.HasPrefix(chanName, "@") {
				chanName = "@" + chanName
			}
			msg := tgbotapi.NewMessageToChannel(chanName, orderText)
			msg.ParseMode = tgbotapi.ModeHTML
			_, err := bot.Send(msg)
			if err != nil {
				log.Printf("❌ Kanalga yuborishda xatolik (%s): %v", chanName, err)
			} else {
				log.Printf("✅ Buyurtma kanalga yuborildi: %s", chanName)
			}
		}
	} else {
		log.Println("⚠️ Kanal ID hali ulanmagan. Kanaldan xabar forward qiling.")
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

func startFlow(bot *tgbotapi.BotAPI, chatID int64, userID int64, username, firstName string) {
	clearState(userID)

	msgText := fmt.Sprintf(
		"👋 <b>Assalomu alaykum, %s!</b>\n\n"+
			"Men <b>Haydarali Akbarov</b>ning rasmiy portfolio botiman. 🚀\n\n"+
			"Bu yerda siz loyihangiz bo'yicha to'liq ma'lumot berishingiz va buyurtma qoldirishingiz mumkin.\n\n"+
			"Keling, boshlaymiz!\n\n"+
			"1️⃣ <b>Iltimos, ismingiz va familiyangizni (yoki kompaniyangiz nomini) kiriting:</b>",
		html.EscapeString(firstName),
	)

	state := getOrCreateState(userID, username)
	state.Step = StepWaitingName

	reply := tgbotapi.NewMessage(chatID, msgText)
	reply.ParseMode = tgbotapi.ModeHTML
	reply.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	_, _ = bot.Send(reply)
}

func startOrder(bot *tgbotapi.BotAPI, chatID int64, userID int64, username string) {
	state := getOrCreateState(userID, username)
	state.Step = StepWaitingName

	msgText := "🚀 <b>Yangi loyiha bo'yicha buyurtma berish</b>\n\n" +
		"1️⃣ <b>Iltimos, ismingiz va familiyangizni (yoki kompaniyangiz nomini) kiriting:</b>"

	reply := tgbotapi.NewMessage(chatID, msgText)
	reply.ParseMode = tgbotapi.ModeHTML
	reply.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
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
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "JSON error"})
		return
	}

	tech := req.Technology
	switch strings.ToLower(tech) {
	case "react", "react.js":
		tech = TechReact
	case "next", "next.js":
		tech = TechNext
	case "vue", "vue.js":
		tech = TechVue
	default:
		tech = "Ko'rsatilmagan"
	}

	state := &UserState{
		Name:        req.Name,
		Phone:       req.Phone,
		Telegram:    req.Telegram,
		ProjectInfo: req.Project,
		Technology:  tech,
		UpdatedAt:   time.Now(),
	}

	nowStr := time.Now().Format("2006-01-02 15:04:05")
	sendOrderRouting(bot, state, 0, nowStr, "Portfolio Web-sayti")

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
			Step:      StepStart,
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
