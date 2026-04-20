package main

import (
	"context"
	"crypto/rand"     // Криптографически безопасные случайные числа
	"encoding/base64" // Преобразование байтов в текст (для короткого кода)
	"encoding/json"   // Превращение Go структур в JSON и обратно
	"fmt"             // Форматирование строк (как printf в C)
	"log"             // Вывод сообщений в консоль с временем
	"net/http"        // Всё для HTTP сервера (запросы, ответы)
	"os"              // Доступ к переменным окружения и аргументам
	"time"            // Работа со временем (таймауты, задержки)

	"github.com/jackc/pgx/v5/pgxpool" // Драйвер PostgreSQL (пул соединений)
	"github.com/joho/godotenv"        // Чтение .env файла
	"github.com/redis/go-redis/v9"    // Клиент для Redis
)

type CreateLinkRequest struct {
	URL string `json:"url"`
}

type CreateLinkResponse struct {
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
	Clicks      int    `json:"clicks"`
}

func generateShortCode() string {
	bytes := make([]byte, 6)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)[:8]
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Файл не найден")
	}

	dbUrl := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)

	db, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		log.Fatal("Ошибка подключения", err)
	}
	defer db.Close()

	if err := db.Ping(context.Background()); err != nil {
		log.Fatal("База не отвечает", err)
	}
	log.Println("База подключена")

	createTableSql := `
	CREATE TABLE IF NOT EXISTS links (
		id SERIAL PRIMARY KEY,
		original_url TEXT NOT NULL,
		short_code VARCHAR(10) UNIQUE NOT NULL,
		clicks INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	CREATE INDEX IF NOT EXISTS idx_short_code ON links(short_code);
	`

	_, err = db.Exec(context.Background(), createTableSql)
	if err != nil {
		log.Fatal("Ошибка создания таблицы", err)
	}
	log.Println("Таблица создана", err)

	rdb := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal("Redis не отвечает", err)
	}
	log.Println("Redis подключена", err)

	http.HandleFunc("/shorten", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Неверный метод", http.StatusMethodNotAllowed)
			return
		}

		var req CreateLinkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.URL == "" {
			http.Error(w, "URL is required", http.StatusBadRequest)
			return
		}

		shortCode := generateShortCode()

		query := `INSERT INTO links (original_url, short_code) VALUES ($1, $2)`
		_, err := db.Exec(context.Background(), query, req.URL, shortCode)
		if err != nil {
			log.Println("Ошибка сохранения")
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		port := os.Getenv("SERVER_PORT")
		shortUrl := fmt.Sprintf("http://localhost:%s/%s", port, shortCode)

		resp := CreateLinkResponse{
			ShortURL:    shortUrl,
			OriginalURL: req.URL,
			Clicks:      0,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		shortCode := r.URL.Path[1:]
		if shortCode == "" {
			w.Write([]byte(`{"status":"URL Shortener API is running"}`))
			return
		}

		cacheKey := "link:" + shortCode
		cachedUrl, err := rdb.Get(context.Background(), cacheKey).Result()
		if err == nil {
			go db.Exec(context.Background(), "UPDATE links SET clicks = clicks + 1 WHERE short_code = $1", shortCode)
			http.Redirect(w, r, cachedUrl, http.StatusFound)
			return
		}

		var originalURL string
		query := `SELECT original_url FROM links WHERE short_code = $1`
		err = db.QueryRow(context.Background(), query, shortCode).Scan(&originalURL)
		if err != nil {
			http.Error(w, "Ссылка не найдена", http.StatusNotFound)
			return
		}
		rdb.Set(context.Background(), cacheKey, originalURL, 5*time.Minute)
		go db.Exec(context.Background(), "UPDATE links SET clicks = clicks + 1 WHERE short_code = $1", shortCode)
		http.Redirect(w, r, originalURL, http.StatusFound)
		return
	})
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Сервер запущен на http://localhost:%s", port)
	log.Printf("Создать ссылку: POST http://localhost:%s/shorten", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
