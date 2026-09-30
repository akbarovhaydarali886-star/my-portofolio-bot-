package bot

import (
	"fmt"
	"html"
	"log"
	"strings"
	"sync"
	"time"

	"portfolio-bot/config"
	"portfolio-bot/telegram"
)

type Step int

const (
	StepIdle Step = iota
	StepWaitingName
	StepWaitingContact
	StepWaitingProject
	StepWaitingTech
)

const (
	TechReact = "⚛️ React.js (Qiyin ishlar uchun)"
	TechNext  = "▲ Next.js (Oson ishlar uchun)"
	TechVue   = "🟢 Vue.js (Oson ishlar uchun)"
)

type UserSession struct {
	Step        Step
	Name        string
	Phone       string
	TelegramTag string
	ProjectInfo string
	Technology  string
	UpdatedAt   time.Time
}

type BotService struct {
	client   *telegram.Client
	cfg      *config.Config
	sessions map[int64]*UserSession
	mu       sync.RWMutex
}

func NewBotService(client *telegram.Client, cfg *config.Config) *BotService {
	return &BotService{
		client:   client,
		cfg:      cfg,
		sessions: make(map[int64]*UserSession),
	}
}

// StartBot registers commands and starts polling loop
func (b *BotService) StartBot() {
	if b.cfg.BotToken == "" {
		log.Println("⚠️ TELEGRAM_BOT_TOKEN is empty. Telegram bot polling will not start.")
		return
	}

	botUser, err := b.client.GetMe()
	if err != nil {
		log.Printf("❌ Failed to connect to Telegram API: %v", err)
		return
	}

	log.Printf("✅ Telegram Bot connected as @%s (ID: %d, Name: %s)", botUser.Username, botUser.ID, botUser.FirstName)

	// Set menu commands
	commands := []telegram.BotCommand{
		{Command: "start", Description: "Botni ishga tushirish / Yangi buyurtma"},
		{Command: "order", Description: "Loyiha bo'yicha buyurtma berish"},
		{Command: "portfolio", Description: "Haydarali Akbarov portfoliosi"},
		{Command: "help", Description: "Yordam va aloqa ma'lumotlari"},
		{Command: "cancel", Description: "Jarayonni bekor qilish"},
	}
	if err := b.client.SetMyCommands(commands); err != nil {
		log.Printf("Warning: failed to set bot commands: %v", err)
	}

	log.Println("🚀 Telegram bot is polling for updates...")

	var offset int64 = 0
	for {
		updates, err := b.client.GetUpdates(offset, 25)
		if err != nil {
			log.Printf("Polling error: %v. Retrying in 3 seconds...", err)
			time.Sleep(3 * time.Second)
			continue
		}

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}

			// 1. Detect channel from ChannelPost (when any message is sent in the channel)
			if update.ChannelPost != nil && update.ChannelPost.Chat != nil {
				b.handleChannelDetection(update.ChannelPost.Chat)
				continue
			}

			// 2. Detect channel from MyChatMember (when bot is added to channel as admin)
			if update.MyChatMember != nil && (update.MyChatMember.Chat.Type == "channel" || update.MyChatMember.Chat.Type == "supergroup") {
				b.handleChannelDetection(&update.MyChatMember.Chat)
				continue
			}

			// 3. User messages
			if update.Message != nil {
				b.handleMessage(update.Message)
			} else if update.CallbackQuery != nil {
				b.handleCallbackQuery(update.CallbackQuery)
			}
		}
	}
}

func (b *BotService) handleChannelDetection(chat *telegram.Chat) {
	if chat == nil {
		return
	}
	channelID := fmt.Sprintf("%d", chat.ID)
	log.Printf("📢 [AUTO-DETECTED CHANNEL] Title: '%s', Username: '%s', ID: %s", chat.Title, chat.Username, channelID)

	// Update configured channel ID
	b.cfg.UpdateChannelID(channelID)

	// Post confirmation in channel
	welcomeMsg := fmt.Sprintf(
		"✅ <b>Haydarali Akbarov Portfolio Boti ulandi!</b> 🚀\n\n"+
			"Ushbu kanal (<b>%s</b>) barcha yangi taklif va buyurtmalarni qabul qilish uchun muvaffaqiyatli sozlandi.",
		html.EscapeString(chat.Title),
	)
	_ = b.client.SendMessage(channelID, welcomeMsg, nil)
}

func (b *BotService) getOrCreateSession(userID int64, username string) *UserSession {
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.sessions[userID]
	if !ok {
		s = &UserSession{
			Step:        StepIdle,
			TelegramTag: username,
			UpdatedAt:   time.Now(),
		}
		b.sessions[userID] = s
	} else if username != "" {
		s.TelegramTag = username
	}
	s.UpdatedAt = time.Now()
	return s
}

func (b *BotService) clearSession(userID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.sessions, userID)
}

func (b *BotService) handleMessage(msg *telegram.Message) {
	if msg == nil || msg.Chat == nil || msg.From == nil {
		return
	}

	chatID := msg.Chat.ID
	userID := msg.From.ID
	username := msg.From.Username
	text := strings.TrimSpace(msg.Text)

	// Check if this message was forwarded from a channel to auto-link the channel
	if msg.ForwardFromChat != nil && (msg.ForwardFromChat.Type == "channel" || msg.ForwardFromChat.Type == "supergroup") {
		channelID := fmt.Sprintf("%d", msg.ForwardFromChat.ID)
		b.cfg.UpdateChannelID(channelID)
		log.Printf("📢 [CHANNEL LINKED VIA FORWARD] Title: %s, ID: %s", msg.ForwardFromChat.Title, channelID)

		confirmMsg := fmt.Sprintf(
			"✅ <b>Taklif kanali muvaffaqiyatli ulandi!</b>\n\n"+
				"📢 <b>Kanal nomi:</b> %s\n"+
				"🆔 <b>Kanal ID:</b> <code>%s</code>\n\n"+
				"Zanjir to'liq sozlandi:\n"+
				"<b>Mijoz so'rovi ➔ Bot ➔ Kanal ➔ Siz</b>\n\n"+
				"Endi har qanday buyurtma to'g'ridan-to'g'ri shu kanalga va shaxsan sizga yuboriladi!",
			html.EscapeString(msg.ForwardFromChat.Title),
			channelID,
		)
		_ = b.client.SendMessage(chatID, confirmMsg, nil)
		return
	}

	// If Haydarali / developer talks to the bot, automatically register them as Admin
	if strings.EqualFold(username, strings.TrimPrefix(b.cfg.DeveloperTG, "@")) || b.cfg.AdminChatID == "" {
		b.cfg.UpdateAdminChatID(fmt.Sprintf("%d", chatID))
	}

	// Admin command: /setchannel
	if strings.HasPrefix(text, "/setchannel") {
		parts := strings.Fields(text)
		if len(parts) >= 2 {
			ch := parts[1]
			b.cfg.UpdateChannelID(ch)
			_ = b.client.SendMessage(chatID, fmt.Sprintf("✅ Kanal ID yangilandi: <code>%s</code>", ch), nil)
			_ = b.client.SendMessage(ch, "✅ Portfolio bot ushbu kanalga ulandi!", nil)
			return
		}
		_ = b.client.SendMessage(chatID, "Foydalanish: <code>/setchannel -100xxxxxxxxxx</code> yoki kanal xabarini bu yerga forward qiling!", nil)
		return
	}

	// Global commands
	switch text {
	case "/start":
		b.cmdStart(chatID, userID, username, msg.From.FirstName)
		return
	case "/order":
		b.startOrderFlow(chatID, userID, username)
		return
	case "/cancel", "❌ Bekor qilish":
		b.clearSession(userID)
		_ = b.client.SendMessage(chatID,
			"❌ <b>Buyurtma berish bekor qilindi.</b>\n\nQayta buyurtma berish uchun /order yoki /start bosing.",
			telegram.ReplyKeyboardRemove{RemoveKeyboard: true},
		)
		return
	case "/help":
		b.cmdHelp(chatID)
		return
	case "/portfolio":
		b.cmdPortfolio(chatID)
		return
	}

	// Handle state machine steps
	session := b.getOrCreateSession(userID, username)

	switch session.Step {
	case StepWaitingName:
		if text == "" {
			_ = b.client.SendMessage(chatID, "Iltimos, ismingiz va familiyangizni matn ko'rinishida yozing:", nil)
			return
		}
		session.Name = text
		session.Step = StepWaitingContact

		contactKeyboard := telegram.ReplyKeyboardMarkup{
			Keyboard: [][]telegram.KeyboardButton{
				{
					{Text: "📱 Telefon raqamni ulashish", RequestContact: true},
				},
				{
					{Text: "❌ Bekor qilish"},
				},
			},
			ResizeKeyboard:  true,
			OneTimeKeyboard: true,
		}

		greeting := fmt.Sprintf(
			"Rahmat, <b>%s</b>!\n\n"+
				"2️⃣ <b>Bog'lanish uchun telefon raqamingizni kiriting:</b>\n"+
				"<i>(Pastdagi '📱 Telefon raqamni ulashish' tugmasini bosishingiz yoki raqamingizni yozib yuborishingiz mumkin)</i>",
			html.EscapeString(session.Name),
		)
		_ = b.client.SendMessage(chatID, greeting, contactKeyboard)

	case StepWaitingContact:
		phone := ""
		if msg.Contact != nil {
			phone = msg.Contact.PhoneNumber
		} else if text != "" {
			phone = text
		}

		if phone == "" {
			_ = b.client.SendMessage(chatID, "Iltimos, telefon raqamingizni kiriting yoki 'Telefon raqamni ulashish' tugmasidan foydalaning:", nil)
			return
		}

		session.Phone = phone
		session.Step = StepWaitingProject

		prompt := "Ajoyib! Endi esa loyiha haqida to'liq ma'lumot berishingizni so'raymiz. 🎯\n\n" +
			"3️⃣ <b>Qanaqa loyiha qilmoqchisiz? Loyiha haqida to'liq informatsiya bering:</b>\n\n" +
			"💡 <b>Tavsiya qilinadigan ma'lumotlar:</b>\n" +
			"• Loyiha maqsadi va turi (Landing page, Korporativ sayt, E-commerce / do'kon, CRM, Web dastur)\n" +
			"• Kerakli asosiy funksiyalar va imkoniyatlar\n" +
			"• Taxminiy muddat va byudjet\n" +
			"• Boshqa qo'shimcha talablar\n\n" +
			"Iltimos, barchasini batafsil yozib qoldiring:"

		_ = b.client.SendMessage(chatID, prompt, telegram.ReplyKeyboardRemove{RemoveKeyboard: true})

	case StepWaitingProject:
		if text == "" {
			_ = b.client.SendMessage(chatID, "Iltimos, loyihangiz haqida batafsil matn yozing:", nil)
			return
		}
		session.ProjectInfo = text
		session.Step = StepWaitingTech

		b.showTechOptions(chatID)

	case StepWaitingTech:
		switch text {
		case TechReact, "React", "React.js", "React JS", "react":
			b.completeOrder(chatID, userID, session, TechReact)
		case TechNext, "Next", "Next.js", "Next JS", "next":
			b.completeOrder(chatID, userID, session, TechNext)
		case TechVue, "Vue", "Vue.js", "Vue JS", "vue":
			b.completeOrder(chatID, userID, session, TechVue)
		default:
			b.showTechOptions(chatID)
		}

	default:
		welcomeMsg := "Assalomu alaykum! Yangi loyiha bo'yicha ariza qoldirish uchun /order yoki /start bosing."
		_ = b.client.SendMessage(chatID, welcomeMsg, nil)
	}
}

func (b *BotService) showTechOptions(chatID int64) {
	prompt := "4️⃣ <b>Loyihangiz uchun qaysi texnologiyani ma'qul ko'rasiz?</b>\n\n" +
		"Quyidagi 3 ta variantdan birini tanlang:\n\n" +
		"• <b>" + TechReact + "</b>\n" +
		"• <b>" + TechNext + "</b>\n" +
		"• <b>" + TechVue + "</b>"

	inlineKeyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: TechReact, CallbackData: "tech:react"},
			},
			{
				{Text: TechNext, CallbackData: "tech:next"},
			},
			{
				{Text: TechVue, CallbackData: "tech:vue"},
			},
		},
	}

	_ = b.client.SendMessage(chatID, prompt, inlineKeyboard)
}

func (b *BotService) handleCallbackQuery(cb *telegram.CallbackQuery) {
	if cb == nil || cb.From == nil {
		return
	}

	userID := cb.From.ID
	chatID := int64(0)
	if cb.Message != nil && cb.Message.Chat != nil {
		chatID = cb.Message.Chat.ID
	}

	_ = b.client.AnswerCallbackQuery(cb.ID, "", false)

	session := b.getOrCreateSession(userID, cb.From.Username)

	var selectedTech string
	switch cb.Data {
	case "tech:react":
		selectedTech = TechReact
	case "tech:next":
		selectedTech = TechNext
	case "tech:vue":
		selectedTech = TechVue
	case "start_order":
		b.startOrderFlow(chatID, userID, cb.From.Username)
		return
	default:
		return
	}

	if session.Name == "" || session.ProjectInfo == "" {
		b.startOrderFlow(chatID, userID, cb.From.Username)
		return
	}

	b.completeOrder(chatID, userID, session, selectedTech)
}

func (b *BotService) completeOrder(chatID int64, userID int64, session *UserSession, selectedTech string) {
	session.Technology = selectedTech
	session.Step = StepIdle

	nowStr := time.Now().Format("2006-01-02 15:04:05")

	tgDisplay := "ko'rsatilmagan"
	if session.TelegramTag != "" {
		tgDisplay = "@" + session.TelegramTag
	}

	// 1. Reply confirmation to client
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
		html.EscapeString(session.Name),
		html.EscapeString(session.Phone),
		html.EscapeString(tgDisplay),
		html.EscapeString(session.Technology),
		html.EscapeString(session.ProjectInfo),
	)

	_ = b.client.SendMessage(chatID, clientSummary, nil)

	// 2. KANALGA VA SIZGA (MEN) YUBORISH
	// Zanjir: botga so'rov -> bot -> kanal -> men
	b.SendOrderToChannelAndAdmin(session, userID, nowStr, "Telegram Bot")

	// 3. Clear session
	b.clearSession(userID)
}

// SendOrderToChannelAndAdmin sends order to both the channel and the developer personal chat
func (b *BotService) SendOrderToChannelAndAdmin(session *UserSession, userID int64, timestamp, source string) {
	orderMessage := fmt.Sprintf(
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
		html.EscapeString(session.Name),
		html.EscapeString(session.Phone),
		formatTelegramUserLink(session.TelegramTag, userID),
		html.EscapeString(session.Technology),
		html.EscapeString(session.ProjectInfo),
		html.EscapeString(timestamp),
		html.EscapeString(source),
	)

	// 1. Send to Telegram Channel (bot -> kanal)
	if b.cfg.ChannelID != "" && b.cfg.ChannelID != "@haydarali_orders" {
		err := b.client.SendMessage(b.cfg.ChannelID, orderMessage, nil)
		if err != nil {
			log.Printf("❌ Kanalga yuborishda xatolik (%s): %v", b.cfg.ChannelID, err)
		} else {
			log.Printf("✅ Buyurtma Telegram kanalga muvaffaqiyatli yuborildi: %s", b.cfg.ChannelID)
		}
	} else {
		log.Println("⚠️ Kanal ID hali aniqlanmagan. Kanalga xabar yuborish yoki xabarni botga forward qilish kerak.")
	}

	// 2. Send to Admin / Developer personally (kanal -> men)
	if b.cfg.AdminChatID != "" && b.cfg.AdminChatID != b.cfg.ChannelID {
		adminAlert := "🔔 <b>Yangi buyurtma kelib tushdi! (Kanalga ham yuborildi)</b>\n\n" + orderMessage
		err := b.client.SendMessage(b.cfg.AdminChatID, adminAlert, nil)
		if err != nil {
			log.Printf("Warning: failed to notify admin personally (%s): %v", b.cfg.AdminChatID, err)
		} else {
			log.Printf("✅ Buyurtma shaxsan sizga (admin: %s) ham yetkazildi", b.cfg.AdminChatID)
		}
	}
}

func formatTelegramUserLink(username string, userID int64) string {
	if username != "" {
		return fmt.Sprintf("@%s (ID: <code>%d</code>)", username, userID)
	}
	if userID > 0 {
		return fmt.Sprintf("<a href=\"tg://user?id=%d\">Foydalanuvchi profili</a> (ID: <code>%d</code>)", userID, userID)
	}
	return "Mavjud emas"
}

func (b *BotService) cmdStart(chatID int64, userID int64, username, firstName string) {
	b.clearSession(userID)

	msg := fmt.Sprintf(
		"👋 <b>Assalomu alaykum, %s!</b>\n\n"+
			"Men <b>Haydarali Akbarov</b>ning rasmiy portfolio botiman. 🚀\n\n"+
			"Bu yerda siz loyihangiz bo'yicha to'liq ma'lumot berishingiz va buyurtma qoldirishingiz mumkin.\n\n"+
			"1️⃣ <b>Iltimos, ismingiz va familiyangizni (yoki kompaniyangiz nomini) kiriting:</b>",
		html.EscapeString(firstName),
	)

	session := b.getOrCreateSession(userID, username)
	session.Step = StepWaitingName

	_ = b.client.SendMessage(chatID, msg, telegram.ReplyKeyboardRemove{RemoveKeyboard: true})
}

func (b *BotService) startOrderFlow(chatID int64, userID int64, username string) {
	session := b.getOrCreateSession(userID, username)
	session.Step = StepWaitingName

	msg := "🚀 <b>Yangi loyiha bo'yicha buyurtma berish</b>\n\n" +
		"1️⃣ <b>Iltimos, ismingiz va familiyangizni (yoki kompaniyangiz nomini) kiriting:</b>"

	_ = b.client.SendMessage(chatID, msg, telegram.ReplyKeyboardRemove{RemoveKeyboard: true})
}

func (b *BotService) cmdHelp(chatID int64) {
	helpText := "ℹ️ <b>Haydarali Akbarov — Portfolio Bot</b>\n\n" +
		"📌 <b>Mavjud buyruqlar:</b>\n" +
		"/start — Botni ishga tushirish\n" +
		"/order — Yangi loyiha buyurtma qilish\n" +
		"/portfolio — Portfolio va ishlar bilan tanishish\n" +
		"/cancel — Jarayonni bekor qilish\n" +
		"/help — Ushbu yordam oynasi\n\n" +
		"💡 <b>Admin uchun:</b> Kanalni ulash uchun kanaldan istalgan xabarni botga forward qiling yoki <code>/setchannel kanal_id</code> yozing.\n\n" +
		"📞 <b>Aloqa:</b>\n" +
		"Telegram: @haydaraliakbarov\n" +
		"Telefon: +998 88 083 19 88"

	_ = b.client.SendMessage(chatID, helpText, nil)
}

func (b *BotService) cmdPortfolio(chatID int64) {
	portfolioText := "🌐 <b>Akbarov Haydarali — Frontend Web Dasturchi</b>\n\n" +
		"🛠 <b>Texnologiyalar:</b>\n" +
		"• " + TechReact + "\n" +
		"• " + TechNext + "\n" +
		"• " + TechVue + "\n\n" +
		"🔗 <b>GitHub:</b> github.com/akbarovhaydarali886-star\n" +
		"🌐 <b>Sayt:</b> " + b.cfg.PortfolioURL + "\n\n" +
		"Buyurtma berish uchun /order bosing!"

	inline := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: "🚀 Yangi buyurtma berish", CallbackData: "start_order"},
			},
			{
				{Text: "🌐 Portfolio saytini ko'rish", URL: b.cfg.PortfolioURL},
			},
		},
	}

	_ = b.client.SendMessage(chatID, portfolioText, inline)
}
