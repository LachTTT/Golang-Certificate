# E-Wallet — Assignment (Part 1 Monolith + Part 2 Microservices)

Go + Gin + PostgreSQL + JWT + bcrypt, sesuai `openapi.yaml` dan `Assignment.pdf`.

```
ewallet-assignment/
├── openapi.yaml                  # spesifikasi dari soal
├── part1-monolith/               # PART 1: satu aplikasi Go, satu database (ewallet)
└── part2-microservices/          # PART 2: dipecah
    ├── gateway/                  #   :8080  satu pintu masuk /api/v1
    ├── core-service/             #   :8081  users, wallet, topup, transfer, history
    ├── payment-service/          #   :8082  /transactions/pay (database sendiri: ewallet_payment)
    └── docker-compose.yml
```

## Konfigurasi database

File `.env` di tiap part sudah berisi: `DB_USER=postgres`, `DB_PASSWORD=heromic20`, `DB_NAME=ewallet`
(+ `JWT_SECRET` acak). **Ganti `DB_USER` / `DB_HOST` / `DB_PORT` jika Postgres kamu berbeda.**
`.env` ada di `.gitignore` — jangan di-commit. Gunakan `.env.example` sebagai template.

## Menjalankan

### Opsi A — Docker (paling mudah; Postgres ikut dibuat)
```bash
cd part1-monolith        # atau part2-microservices
docker compose up --build
# API: http://localhost:8080/api/v1
```
Postgres di container dipublish ke port host **5433** (supaya tidak bentrok dengan Postgres lokal).

### Opsi B — Pakai Postgres lokal kamu (DB `ewallet`)
```bash
# Part 1
cd part1-monolith
go mod tidy
set -a; . ./.env; set +a
go run ./cmd/server                      # :8080

# Part 2 (buat dulu database kedua)
psql -U postgres -c "CREATE DATABASE ewallet_payment;"
cd part2-microservices
set -a; . ./.env; set +a
(cd core-service    && go mod tidy && PORT=8081 go run ./cmd/server) &
(cd payment-service && go mod tidy && PORT=8082 go run ./cmd/server) &
(cd gateway         && PORT=8080 go run .) &
```
Tabel dibuat otomatis saat service start (auto-migrate).

## Endpoint

| Method | Path | Auth | Part 1 | Part 2 |
|---|---|---|---|---|
| POST | `/api/v1/users/register` | – | monolith | core |
| POST | `/api/v1/users/login` | – | monolith | core |
| GET | `/api/v1/wallet/balance` | JWT | monolith | core |
| POST | `/api/v1/transactions/topup` | JWT | monolith | core |
| POST | `/api/v1/transactions/transfer` | JWT | monolith | core |
| GET | `/api/v1/transactions/history?page=&limit=` | JWT | monolith | core |
| POST | `/api/v1/transactions/pay` | JWT | monolith | **payment** |
| GET | `/api/v1/transactions/pay/{id}` *(tambahan)* | JWT | monolith | **payment** |
| POST | `/api/v1/transactions/pay/{id}/confirm` *(tambahan)* | JWT | monolith | **payment** |

## Expiring Payments (10 menit)

`POST /transactions/pay` membuat **payment session** (status `pending`, `expiresAt = sekarang + 10 menit`).
Saldo **belum** dipotong. Pembayaran diselesaikan lewat `POST /transactions/pay/{id}/confirm`.

- **Cek saat confirm:** baris session dikunci (`SELECT … FOR UPDATE`), lalu `expires_at <= now()` dicek memakai jam database.
  Jika lewat → status `expired`, respons **410 Gone**, saldo tidak berubah.
- **Background job:** goroutine tiap 30 detik menandai session yang lewat batas sebagai `expired`.
- Di `/transactions/history`, `expired` ditampilkan sebagai `failed` (enum OpenAPI hanya `completed|pending|failed`).
- TTL bisa diubah lewat `PAYMENT_TTL_MINUTES` (untuk uji cepat, set `1`).

Respons `/pay` tetap berbentuk `PayResponse` (`transactionID`, `message`, `newBalance`) + field tambahan `status` dan `expiresAt`.

## Keputusan desain

- **Uang = `NUMERIC(18,2)`** + `CHECK (balance >= 0)`; amount divalidasi ≥ 1 dan maksimal 2 desimal.
- **Transfer** mengikuti OpenAPI (`202 pending`): divalidasi dulu (penerima ada, saldo cukup → jika tidak `400`), disimpan `pending`,
  lalu diselesaikan async dalam satu DB transaction. Kedua wallet dikunci **berurutan by user_id** agar tidak deadlock.
  Transfer `pending` yang nyangkut (mis. server mati) di-retry oleh background job.
- **Part 2 – komunikasi service:** `payment-service` memvalidasi JWT sendiri (secret yang sama), lalu memanggil API internal
  core-service (`GET /internal/wallets/{id}/balance`, `POST /internal/wallets/debit`) dengan header `X-Internal-Token`.
  Route `/internal/*` **tidak** diteruskan oleh gateway dan core-service tidak dipublish ke host.
- **Idempotent debit:** `reference` = ID payment session. Debit yang diulang tidak memotong dua kali, jadi aman kalau
  confirm di-retry setelah timeout/crash. Debit juga dicatat di tabel `transactions` core, sehingga payment muncul di history.
- **Database per service:** `payment-service` hanya memakai DB `ewallet_payment` (tabel `payment_sessions`).
- **Keamanan:** bcrypt, JWT HS256 (algoritma dipaksa), email di-lowercase, login timing-safe, error 409 untuk email ganda,
  password dibatasi 8–72 karakter (batas bcrypt).

## Contoh cepat

```bash
B=http://localhost:8080/api/v1
curl -X POST $B/users/register -H 'Content-Type: application/json' \
  -d '{"name":"Alice","email":"alice@example.com","password":"password123"}'
TOKEN=$(curl -s -X POST $B/users/login -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123"}' | jq -r .token)

curl -X POST $B/transactions/topup -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"amount":1000000}'

# buat payment session lalu confirm
PID=$(curl -s -X POST $B/transactions/pay -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"merchantID":"merchant-123","amount":100000,"description":"Coffee"}' | jq -r .transactionID)
curl -X POST $B/transactions/pay/$PID/confirm -H "Authorization: Bearer $TOKEN"
```

## Catatan jujur
- Part 1 dan ketiga service Part 2 sudah di-compile, di-`go vet`, dan diuji end-to-end dengan PostgreSQL 16
  (register/login, topup, transfer async, pay→confirm, expired via confirm & via background job, debit idempotent,
  `/internal` tidak terekspos gateway).
- `go.sum` tidak disertakan; jalankan `go mod tidy` (Dockerfile sudah melakukannya). File Docker/compose belum sempat dijalankan di lingkungan saya.
- Belum ada unit test otomatis & rate limiting; itu langkah lanjutan yang wajar.
