# Refa Optical Backend

REST API untuk storefront Refa Optical, dibangun dengan Go, Gin, GORM, dan MySQL. Payment gateway memakai Midtrans Snap. File resep disimpan privat di disk pada development, dengan adapter S3-compatible opsional untuk produksi.

## Menjalankan lokal

Prasyarat: Go 1.26+ dan Laragon dengan MySQL aktif. Midtrans Server Key sandbox diperlukan saat mencoba pembayaran.

1. Buka Laragon dan start MySQL. Buat database bernama `refa_optical` melalui HeidiSQL/phpMyAdmin atau tool database pilihan Anda.
2. Salin `.env.example` menjadi `.env`. DSN bawaan memakai akun Laragon yang umum (`root`, tanpa password, port `3306`):

   ```env
   DATABASE_DSN=root:@tcp(127.0.0.1:3306)/refa_optical?charset=utf8mb4&parseTime=True&loc=Local
   ```

   Sesuaikan username, password, atau port jika konfigurasi MySQL Laragon Anda berbeda. Contoh dengan password: `refa:password@tcp(127.0.0.1:3307)/refa_optical?charset=utf8mb4&parseTime=True&loc=Local`.
3. Isi `JWT_SECRET` dengan nilai acak minimal 32 karakter dan ubah password admin development. `MIDTRANS_SERVER_KEY` boleh diisi saat Anda mulai menguji pembayaran.
4. Jalankan API dari folder `backend`:

   ```powershell
   go mod tidy
   go run ./cmd/api
   ```

Alternatifnya, MySQL dapat dijalankan dengan `docker compose up -d mysql` bila tidak memakai service MySQL Laragon.

### Menggunakan TiDB Cloud

Untuk koneksi publik TiDB Cloud, tambahkan IP komputer/server backend ke IP Access List TiDB. Dari dialog **Connect**, buat password database bila belum ada, pilih database aplikasi (misalnya `refa_optical`), lalu ambil host, port, dan username persis dari tab **Connection String**. Jangan menaruh password di kode atau membagikannya.

Atur `.env` backend dengan format DSN driver Go MySQL berikut; ganti nilai dalam kurung dengan informasi TiDB Anda:

```env
DATABASE_DSN=<username>:<password>@tcp(<host>:4000)/refa_optical?charset=utf8mb4&parseTime=True&loc=Local
DATABASE_CA_CERT_PATH=C:/Users/Renji/Projects/final-project/backend/certs/ca.pem
```

Unduh CA certificate dari dialog **Connect**, simpan sebagai file lokal (contoh `backend/certs/ca.pem`), lalu sesuaikan `DATABASE_CA_CERT_PATH`. Pastikan file `.env` dan sertifikat tidak di-commit. Backend akan memakai TLS dengan verifikasi sertifikat server saat path CA diisi; jika CA tidak diisi, perilaku koneksi lokal Laragon tetap seperti sebelumnya. Pilih database aplikasi, bukan `sys`, agar tabel toko tidak dibuat di database sistem. Setelah menyimpan `.env`, jalankan ulang API.

API tersedia di `http://localhost:8080`; health check di `/healthz`. Saat startup, GORM membuat/memperbarui tabel model, migrasi memperbaiki nilai kosong/duplikat pada kolom unik serta menyelaraskan foreign key legacy tanpa menghapus baris, lalu mengisi tiga produk contoh jika tabel katalog masih kosong. Admin dibuat dari `ADMIN_EMAIL` dan `ADMIN_PASSWORD` jika belum ada; jika alamat tersebut sudah terdaftar, role-nya disetel menjadi admin.

> Jangan gunakan kredensial contoh atau `GIN_MODE=debug` untuk produksi. Simpan `.env` dan Midtrans Server Key di secret manager, gunakan HTTPS, ganti `JWT_SECRET`, dan set `COOKIE_SECURE=true`.

## Cakupan API

Semua route bisnis berada di `/api/v1`. Respons sukses memakai `{ "data": ... }`; error memakai `{ "error": { "code", "message" } }`.

| Method | Endpoint | Akses | Tujuan |
|---|---|---|---|
| GET | `/products` | Publik | Katalog; filter `query`, `gender`, `face_type`, `frame`, `category`, `min_price`, `max_price`, `sort` |
| GET | `/products/:slug` | Publik | Detail produk |
| POST | `/auth/register` | Publik | Buat akun pelanggan |
| POST | `/auth/login` | Publik | Login, memberi access token dan refresh cookie HttpOnly |
| POST | `/auth/refresh` | Cookie | Rotasi refresh token dan buat access token baru |
| POST | `/auth/logout` | Publik | Cabut refresh token dan hapus cookie |
| GET | `/auth/me` | Bearer | Profil akun aktif |
| POST | `/orders` | Bearer | Validasi harga/stok dari DB dan buat pesanan |
| GET | `/orders` | Bearer | Riwayat milik pelanggan aktif |
| GET | `/orders/:id` | Bearer | Detail pesanan milik pelanggan aktif |
| POST | `/orders/:id/prescription` | Bearer | Upload file resep (`multipart/form-data`, field `file`) |
| POST | `/orders/:id/payments` | Bearer | Buat/mengambil transaksi Midtrans Snap |
| POST | `/payments/midtrans/notification` | Midtrans | Webhook notifikasi transaksi |
| GET/POST | `/admin/products` | Admin | Daftar produk/produk baru |
| PUT/DELETE | `/admin/products/:id` | Admin | Ubah/nonaktifkan produk |
| PATCH | `/admin/products/:id/stock` | Admin | Sesuaikan stok dengan `{ "delta": 5 }` |
| GET | `/admin/orders` | Admin | Daftar pesanan, dapat difilter dengan `status` |
| PATCH | `/admin/orders/:id` | Admin | Perbarui status ke `processing`, `completed`, atau `cancelled` |
| GET | `/admin/orders/:id/prescription` | Admin | Unduh resep melalui endpoint yang dilindungi |

Contoh body pembuatan order:

```json
{
  "items": [{ "productId": "<id-produk>", "quantity": 1, "colorName": "Onyx" }],
  "address": {
    "name": "Nama Pelanggan",
    "phone": "081234567890",
    "address": "Jalan Contoh 1",
    "city": "Jakarta",
    "postalCode": "10000"
  }
}
```

Untuk order kacamata minus, urutannya: buat order, upload resep, lalu buat pembayaran. Order tanpa resep tidak dapat memulai pembayaran.

## Menghubungkan frontend

Frontend memakai HTTP client di `src/api` dengan base URL `/api/v1`. Vite meneruskan `/api` ke `http://localhost:8080` saat development. Untuk server frontend lain, atur `VITE_API_BASE_URL` (contoh `https://api.example.com/api/v1`) dan masukkan origin frontend ke `CORS_ORIGINS` backend. Client menyertakan access token Bearer serta cookie HttpOnly, dan mencoba refresh cookie ketika access token kedaluwarsa.

Saat checkout, frontend mengirim order ke `POST /orders`, mengunggah resep melalui endpoint resep jika diperlukan, lalu memanggil `POST /orders/:id/payments` dan mengarahkan pelanggan ke `redirectUrl`. Status pembayaran dan pesanan dibaca dari API; redirect browser bukan konfirmasi pembayaran. Adapter `src/api/admin.ts` juga menyediakan akses ke endpoint pengelolaan produk, stok, pesanan, dan lampiran resep untuk UI admin mendatang.

## Midtrans

Isi `MIDTRANS_SERVER_KEY` dengan Server Key dari dashboard Midtrans. Default memakai Snap Sandbox; set `MIDTRANS_IS_PRODUCTION=true` hanya dengan kredensial produksi. Daftarkan `https://<domain-api>/api/v1/payments/midtrans/notification` sebagai Payment Notification URL di dashboard Midtrans. Backend membuat token transaksi dari server dan memverifikasi signature SHA-512 sebelum mengubah status pesanan. Status `settlement`, atau `capture` dengan fraud status aman, dianggap pembayaran berhasil.

Jangan kirim Server Key ke browser. Integrasi memakai Snap Redirect URL dari respons Snap, jadi halaman web tidak membutuhkan Snap.js untuk alur redirect.

## Penyimpanan resep

- Development: `STORAGE_DRIVER=local`; file disimpan di `UPLOAD_DIR` dengan permission privat. Direktori tersebut tidak disajikan sebagai static assets.
- Produksi S3-compatible: set `STORAGE_DRIVER=s3`, `S3_ENDPOINT`, `S3_REGION`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, dan `S3_USE_SSL`. Adapter menggunakan MinIO Go SDK yang mendukung object storage S3-compatible.
- Upload membatasi ukuran sesuai `MAX_PRESCRIPTION_BYTES` (default 5 MiB) serta memeriksa MIME hasil deteksi file (JPEG, PNG, PDF). File hanya dapat diunduh oleh admin melalui endpoint berautentikasi.

## Catatan implementasi

- Cart pelanggan tetap berada di frontend; API menghitung ulang harga, memeriksa status produk, dan mengurangi stok secara transaksional ketika order dibuat.
- Stok yang dicadangkan dikembalikan satu kali saat Midtrans mengirim status failed/expired/cancelled, atau ketika admin membatalkan order.
- Refresh token disimpan dalam bentuk hash di MySQL dan diputar saat refresh. Access token memakai JWT HS256 dengan masa berlaku pendek.
- Upload resep, database, akun, dan gateway belum diuji dengan kredensial eksternal pada tahap ini; gunakan Midtrans Sandbox sebelum mengaktifkan produksi.
- Ongkos kirim untuk sementara `0` dan belum diintegrasikan.



## Deploy ke Vercel (Go Functions)

Backend ini menyediakan `api/index.go` sebagai Vercel Function. `vercel.json` meneruskan `/api/*` dan `/healthz` ke handler Gin; `go run ./cmd/api` tetap dipakai untuk menjalankan server lokal.

Di pengaturan proyek Vercel, set **Root Directory** ke root repository backend. Tambahkan environment variables produksi yang dibutuhkan aplikasi, minimal `DATABASE_DSN`, `DATABASE_CA_CERT_PATH=certs/isrgrootx1.pem`, `JWT_SECRET` (minimal 32 karakter), dan `GIN_MODE=release`. Tambahkan juga `CORS_ORIGINS` dan `FRONTEND_URL` dengan URL frontend produksi, `COOKIE_SECURE=true`, serta kredensial Midtrans. Jangan masukkan `.env` ke Git.

Untuk upload resep yang persisten, gunakan `STORAGE_DRIVER=s3` beserta `S3_ENDPOINT`, `S3_REGION`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, dan `S3_USE_SSL`. Penyimpanan lokal pada Vercel hanya memakai `/tmp` dan bersifat sementara.

Setelah mengatur env, lakukan redeploy. Cek `https://<domain-backend>/healthz`, lalu cek Vercel Runtime Logs jika Function mengembalikan 503/500. Runtime Go Vercel berstatus Beta.
