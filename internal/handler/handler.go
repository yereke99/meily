package handler

import (
	"context"
	"fmt"
	"io"
	"meily/config"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
)

const (
	stateStart string = "start"
	stateCount string = "count"
	statePaid  string = "paid"
)

type UserState struct {
	State  string
	Count  int
	IsPaid bool
}

type Handler struct {
	cfg    *config.Config
	logger *zap.Logger
	ctx    context.Context
	state  map[int64]*UserState
}

func NewHandler(cfg *config.Config, zapLogger *zap.Logger, ctx context.Context) *Handler {
	return &Handler{
		cfg:    cfg,
		logger: zapLogger,
		ctx:    ctx,
		state:  make(map[int64]*UserState),
	}
}

func (h *Handler) StartHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	fmt.Println("Start state", update.Message.From.ID)

	promoText := "20 000 теңгеге косметикалық жиынтық сатып алыңыз және сыйлықтар ұтып алыңыз!"

	inlineKbd := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text:         "🛍 Сатып алу",
					CallbackData: "buy_cosmetics",
				},
			},
		},
	}
	_, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:         update.Message.Chat.ID,
		Photo:          &models.InputFileString{Data: h.cfg.StartPhotoId},
		Caption:        promoText,
		ReplyMarkup:    inlineKbd,
		ProtectContent: true,
	})
	if err != nil {
		h.logger.Warn("Failed to send promo photo", zap.Error(err))
	}
}

func (h *Handler) CountHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !strings.HasPrefix(update.CallbackQuery.Data, "count_") {
		return
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	choice := strings.Split(update.CallbackQuery.Data, "_")
	if len(choice) != 2 {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Неверный формат данных",
		})
		return
	}
	userCount, err := strconv.Atoi(choice[1])
	if err != nil {
		h.logger.Warn("Failed to parse count", zap.Error(err))
		return
	}

	userID := update.CallbackQuery.From.ID
	h.state[userID] = &UserState{
		State:  statePaid,
		Count:  userCount,
		IsPaid: false,
	}

	inlineKbd := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text: "💳 Төлем жасау",
					URL:  "https://pay.kaspi.kz/pay/ndy27jz5",
				},
			},
		},
	}

	_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      userID,
		Text:        "✅ Тамаша! Енді төмендегі сілтемеге өтіп төлем жасап, төлемді растайтын чекті PDF форматында жіберіңіз.",
		ReplyMarkup: inlineKbd,
	})
	if sendErr != nil {
		h.logger.Warn("Failed to send confirmation message", zap.Error(sendErr))
	}
}

func (h *Handler) PaidHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Document == nil {
		return
	}

	doc := update.Message.Document
	if !strings.EqualFold(filepath.Ext(doc.FileName), ".pdf") {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Қате! Тек қана PDF форматындағы файлдарды қабылдаймыз.",
		})
		return
	}

	userID := update.Message.From.ID
	fileInfo, err := b.GetFile(ctx, &bot.GetFileParams{
		FileID: doc.FileID,
	})
	if err != nil {
		h.logger.Error("Failed to get file info", zap.Error(err))
		return
	}

	// Составляем URL для загрузки через HTTP
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.Token, fileInfo.FilePath)
	resp, err := http.Get(fileURL)
	if err != nil {
		h.logger.Error("Failed to download file via HTTP", zap.Error(err))
		return
	}
	defer resp.Body.Close()

	saveDir := h.cfg.SavePaymentsDir
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		h.logger.Error("Failed to create payments directory", zap.Error(err))
		return
	}
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%d_%s.pdf", update.Message.From.ID, timestamp)
	savePath := filepath.Join(saveDir, filename)

	outFile, err := os.Create(savePath)
	if err != nil {
		h.logger.Error("Failed to create file on disk", zap.Error(err))
		return
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		h.logger.Error("Failed to save PDF file", zap.Error(err))
		return
	}

	state, ok := h.state[userID]
	if ok {
		state.IsPaid = true
		h.state[userID] = state
	}

	userData := fmt.Sprintf("UserID: %d, State: %s, Count: %d, IsPaid: %t", update.Message.From.ID, state.State, state.Count, state.IsPaid)
	h.logger.Info(userData)

	// Подтверждаем получение чека
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "✅ Чек PDF сәтті қабылданды! Рахмет.",
	})
	if err != nil {
		h.logger.Warn("Failed to send confirmation message", zap.Error(err))
	}

	delete(h.state, userID)
}

func (h *Handler) DefaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	var userID int64
	if update.Message != nil {
		userID = update.Message.From.ID
	} else {
		userID = update.CallbackQuery.From.ID
	}

	if userID == h.cfg.AdminID {
		var fileId string
		switch {
		case len(update.Message.Photo) > 0:
			fileId = update.Message.Photo[len(update.Message.Photo)-1].FileID
		case update.Message.Video != nil:
			fileId = update.Message.Video.FileID
		}
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: h.cfg.AdminID,
			Text:   fileId,
		})
		if err != nil {
			h.logger.Error("error send fileId to admin", zap.Error(err))
		}
	}

	userState, ok := h.state[userID]
	if !ok {
		userState = &UserState{
			State:  stateStart,
			Count:  0,
			IsPaid: false,
		}
		h.state[userID] = userState
	}

	if update.CallbackQuery != nil {
		switch userState.State {
		case stateStart:
			h.StartHandler(ctx, b, update)
		case stateCount:
			h.CountHandler(ctx, b, update)
		case statePaid:
			h.PaidHandler(ctx, b, update)
		default:
			h.StartHandler(ctx, b, update)
		}
		return
	}

	switch userState.State {
	case stateStart:
		h.StartHandler(ctx, b, update)
	case stateCount:
		h.CountHandler(ctx, b, update)
	case statePaid:
		h.PaidHandler(ctx, b, update)
	default:
		h.StartHandler(ctx, b, update)
	}
}

func (h *Handler) BuyCosmeticsCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.Data != "buy_cosmetics" {
		return
	}

	userID := update.CallbackQuery.From.ID

	h.state[userID] = &UserState{
		State:  stateCount,
		Count:  0,
		IsPaid: false,
	}

	rows := make([][]models.InlineKeyboardButton, 6)
	for i := 0; i < 6; i++ {
		row := make([]models.InlineKeyboardButton, 5)
		for j := 0; j < 5; j++ {
			num := i*5 + j + 1
			row[j] = models.InlineKeyboardButton{
				Text:         strconv.Itoa(num),
				CallbackData: fmt.Sprintf("count_%d", num),
			}
		}
		rows[i] = row
	}

	btn := &models.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}

	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
	if err != nil {
		h.logger.Warn("Failed to answer callback query", zap.Error(err))
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      userID,
		Text:        "🧴 Косметика санын таңдаңыз 🧴",
		ReplyMarkup: btn,
	})
	if err != nil {
		h.logger.Warn("Failed to answer callback query", zap.Error(err))
	}
}

func (h *Handler) StartWebServer(ctx context.Context, b *bot.Bot) {

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "Meily Bot API")
	})

	if err := http.ListenAndServe(h.cfg.Port, nil); err != nil {
		h.logger.Fatal("failed to start we server", zap.Error(err))
	}
}
