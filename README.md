# 🤖 Haydarali Akbarov Portfolio — Telegram Bot & Backend (Golang)

Ushbu backend va Telegram bot portfolio egasi (Haydarali Akbarov) uchun buyurtmalarni qabul qilish va ularni avtomatik tarzda Telegram kanalga yuborish uchun yaratilgan.

---

## 🌟 Bot qanday ishlaydi?

1. **Mijoz start bosadi**: Bot mijozni samimiy kutib oladi.
2. **1-qadam (Ism)**: Mijoz o'z ism-familiyasi yoki kompaniya nomini kiritadi.
3. **2-qadam (Aloqa)**: Telefon raqamini yozadi yoki `📱 Telefon raqamni ulashish` tugmasi orqali yuboradi.
4. **3-qadam (Loyiha haqida to'liq ma'lumot)**: Qanaqa loyiha kerakligi, talablar, muddat va byudjet haqida to'liq ma'lumot qoldiradi.
5. **4-qadam (3 ta texnologiya varianti)**:
   - ⚛️ **React.js (Qiyin ishlar uchun)**
   - ▲ **Next.js (Oson ishlar uchun)**
   - 🟢 **Vue.js (Oson ishlar uchun)**
6. **Kanalga tashlash**: Mijoz texnologiyani tanlashi bilan buyurtma to'liq shakllanadi va **avtomatik ravishda belgilangan Telegram kanalga** yuboriladi!
7. **Mijozga xulosa**: Mijozga barcha ma'lumotlar xulosa qilinib, tez orada Haydarali bog'lanishi haqida xabar beriladi.

---

## ⚙️ Sozlash (Configuration)

`backend/.env` faylini yarating yoki mavjud namunadan nusxa oling:

```bash
cp .env.example .env
```

`.env` tarkibi:

```env
# 1. @BotFather orqali olingan bot tokeni
TELEGRAM_BOT_TOKEN=1234567890:ABCdefGHIjklMNOpqrsTUVwxyz

# 2. Xabarlar tashlanadigan Telegram Kanal
# Masalan: @loyiha_buyurtmalari yoki -1001234567890
TELEGRAM_CHANNEL_ID=@loyiha_buyurtmalari

# 3. Sizning shaxsiy Chat ID (ixtiyoriy)
TELEGRAM_ADMIN_CHAT_ID=

# 4. Port (default 8080)
PORT=8080
```

### 📢 Telegram Kanalni sozlash (MUHIM):
1. Telegramda ommaviy (Public) yoki xususiy (Private) **Kanal** oching.
2. Botni kanalingiz a'zolariga qo'shing va unga **Administrator (Admin)** huquqini bering.
3. Botingizga **"Post Messages" (Xabarlar yozish)** ruxsatini yoqing.
4. Agar kanal public bo'lsa, uning `@username` sini (masalan: `@haydarali_buyurtmalar`) `.env` dagi `TELEGRAM_CHANNEL_ID` ga yozing.
5. Agar kanal private bo'lsa, kanalning raqamli ID sini (masalan: `-100...`) yozing.

---

## 🚀 Ishga tushirish

Go o'rnatilgan bo'lishi kerak (`go version`).

### Rivojlanish rejimida:
```bash
cd backend
go run main.go
```

### Ishga tayyor fayl (Build) qilish:
```bash
cd backend
go build -o bot.exe main.go
./bot.exe
```

---

## 🌐 Web Backend API (REST)

Backend nafaqat Telegram botni boshqaradi, balki HTTP API ham taqdim etadi:

- `GET /health` — Tizim holatini tekshirish
- `POST /api/order` — Portfolio saytidagi formadan yuborilgan buyurtmalarni ham to'g'ridan-to'g'ri Telegram kanalga yuborish:
  ```json
  {
    "name": "Ali Valiyev",
    "phone": "+998901234567",
    "telegram": "@alivaliyev",
    "project": "Landing page va CRM kerak",
    "technology": "react"
  }
  ```
