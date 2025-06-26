package main

import (
	"context"
	"database/sql"
	"meily/config"
	"meily/internal/handler"
	"meily/internal/repository"
	"meily/traits/database"
	"meily/traits/logger"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-telegram/bot"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

func main() {
	zapLogger, err := logger.NewLogger()
	if err != nil {
		panic(err)
	}

	cfg, err := config.NewConfig()
	if err != nil {
		zapLogger.Error("error initializing config", zap.Error(err))
		return
	}

	db, err := sql.Open("sqlite3", cfg.DBName)
	if err != nil {
		zapLogger.Error("error in connect to database", zap.Error(err))
		return
	}
	defer db.Close()

	if err := database.CreateTables(db); err != nil {
		zapLogger.Error("error in create tables", zap.Error(err))
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	redisClient, err := database.ConnectRedis(ctx, zapLogger)
	if err != nil {
		zapLogger.Error("error connecting to Redis", zap.Error(err))
		return
	}
	defer database.CloseRedis(redisClient, zapLogger)

	userRepo := repository.NewUserRepository(db)
	redisRepo := repository.NewRedisRepository(redisClient)
	handl := handler.NewHandler(cfg, zapLogger, ctx, userRepo, redisRepo)

	opts := []bot.Option{
		bot.WithDefaultHandler(handl.DefaultHandler),
		bot.WithCallbackQueryDataHandler("buy_cosmetics", bot.MatchTypePrefix, handl.BuyCosmeticsCallbackHandler),
		bot.WithCallbackQueryDataHandler("count_", bot.MatchTypePrefix, handl.CountHandler),

		bot.WithMessageTextHandler("/admin", bot.MatchTypeExact, handl.AdminHandler),
		bot.WithMessageTextHandler("💰 Ақша (Money)", bot.MatchTypeExact, handl.AdminHandler),
		bot.WithMessageTextHandler("👥 Тіркелгендер (Just Clicked)", bot.MatchTypeExact, handl.AdminHandler),
		bot.WithMessageTextHandler("🛍 Клиенттер (Clients)", bot.MatchTypeExact, handl.AdminHandler),
		bot.WithMessageTextHandler("🎲 Лото (Loto)", bot.MatchTypeExact, handl.AdminHandler),
		bot.WithMessageTextHandler("📢 Хабарлама (Messages)", bot.MatchTypeExact, handl.AdminHandler),
		bot.WithMessageTextHandler("🎁 Сыйлық (Gift)", bot.MatchTypeExact, handl.AdminHandler),
		bot.WithMessageTextHandler("📊 Статистика (Statistics)", bot.MatchTypeExact, handl.AdminHandler),
		bot.WithMessageTextHandler("❌ Жабу (Close)", bot.MatchTypeExact, handl.AdminHandler),
	}

	b, err := bot.New(cfg.Token, opts...)
	if err != nil {
		zapLogger.Error("error in start bot", zap.Error(err))
		return
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGINT)

	go func() {
		<-stop
		zapLogger.Info("Bot stoppped successfully")
		cancel()
	}()

	go handl.StartWebServer(ctx, b)
	zapLogger.Info("Starting web server", zap.String("port", cfg.Port))
	zapLogger.Info("Bot started successfully")
	b.Start(ctx)
}
