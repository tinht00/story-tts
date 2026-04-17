package telegram

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	gotelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"rsc.io/qr"

	"story-tts/backend/internal/config"
	"story-tts/backend/internal/model"
)

type Manager struct {
	cfg config.TelegramConfig
}

type SendCodeResult struct {
	Phone         string `json:"phone"`
	PhoneCodeHash string `json:"phoneCodeHash"`
	Type          string `json:"type"`
}

func NewManager(cfg config.TelegramConfig) (*Manager, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.SessionFile), 0o755); err != nil {
		return nil, err
	}
	return &Manager{cfg: cfg}, nil
}

func (m *Manager) SessionFile() string {
	return m.cfg.SessionFile
}

func (m *Manager) IsConfigured() bool {
	return m.cfg.AppID > 0 && strings.TrimSpace(m.cfg.AppHash) != ""
}

func (m *Manager) SendCode(ctx context.Context, phone string) (SendCodeResult, error) {
	if !m.IsConfigured() {
		return SendCodeResult{}, fmt.Errorf("telegram app id/app hash chua duoc cau hinh")
	}

	var result SendCodeResult
	err := m.run(ctx, func(client *gotelegram.Client) error {
		sent, err := client.Auth().SendCode(ctx, phone, auth.SendCodeOptions{})
		if err != nil {
			return err
		}
		phoneCodeHash := ""
		switch value := sent.(type) {
		case interface{ GetPhoneCodeHash() string }:
			phoneCodeHash = value.GetPhoneCodeHash()
		}
		result = SendCodeResult{
			Phone:         phone,
			PhoneCodeHash: phoneCodeHash,
			Type:          fmt.Sprintf("%T", sent),
		}
		return nil
	})
	return result, err
}

func (m *Manager) SignIn(ctx context.Context, phone, phoneCode, phoneCodeHash string) error {
	if !m.IsConfigured() {
		return fmt.Errorf("telegram app id/app hash chua duoc cau hinh")
	}
	return m.run(ctx, func(client *gotelegram.Client) error {
		_, err := client.Auth().SignIn(ctx, phone, phoneCode, phoneCodeHash)
		return err
	})
}

func (m *Manager) Password(ctx context.Context, password string) error {
	if !m.IsConfigured() {
		return fmt.Errorf("telegram app id/app hash chua duoc cau hinh")
	}
	return m.run(ctx, func(client *gotelegram.Client) error {
		_, err := client.Auth().Password(ctx, password)
		return err
	})
}

func (m *Manager) RunQRLogin(ctx context.Context, onToken func(snapshot model.TelegramQRLogin)) (model.TelegramAccount, error) {
	if !m.IsConfigured() {
		return model.TelegramAccount{}, fmt.Errorf("telegram app id/app hash chua duoc cau hinh")
	}

	dispatcher := tg.NewUpdateDispatcher()
	loggedIn := qrlogin.OnLoginToken(dispatcher)
	client := gotelegram.NewClient(m.cfg.AppID, m.cfg.AppHash, gotelegram.Options{
		SessionStorage: &gotelegram.FileSessionStorage{Path: m.cfg.SessionFile},
		UpdateHandler:  dispatcher,
		Device: gotelegram.DeviceConfig{
			DeviceModel:    m.cfg.DeviceModel,
			SystemVersion:  m.cfg.SystemVer,
			AppVersion:     m.cfg.AppVersion,
			LangCode:       m.cfg.LangCode,
			SystemLangCode: m.cfg.LangCode,
		},
	})

	var account model.TelegramAccount
	err := client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if status.Authorized && status.User != nil {
			account = telegramAccountFromUser(m.cfg.SessionFile, status.User)
			return nil
		}

		authorization, err := client.QR().Auth(ctx, loggedIn, func(ctx context.Context, token qrlogin.Token) error {
			snapshot, err := qrLoginSnapshot(token)
			if err != nil {
				return err
			}
			onToken(snapshot)
			return nil
		})
		if err != nil {
			return err
		}

		user, ok := authorization.User.AsNotEmpty()
		if !ok {
			return fmt.Errorf("unexpected type %T", authorization.User)
		}
		account = telegramAccountFromUser(m.cfg.SessionFile, user)
		return nil
	})
	return account, err
}

func (m *Manager) run(ctx context.Context, fn func(client *gotelegram.Client) error) error {
	client := gotelegram.NewClient(m.cfg.AppID, m.cfg.AppHash, gotelegram.Options{
		SessionStorage: &gotelegram.FileSessionStorage{Path: m.cfg.SessionFile},
		Device: gotelegram.DeviceConfig{
			DeviceModel:    m.cfg.DeviceModel,
			SystemVersion:  m.cfg.SystemVer,
			AppVersion:     m.cfg.AppVersion,
			LangCode:       m.cfg.LangCode,
			SystemLangCode: m.cfg.LangCode,
		},
	})
	return client.Run(ctx, func(ctx context.Context) error {
		return fn(client)
	})
}

func qrLoginSnapshot(token qrlogin.Token) (model.TelegramQRLogin, error) {
	imageData, err := token.Image(qr.M)
	if err != nil {
		return model.TelegramQRLogin{}, err
	}

	var payload bytes.Buffer
	if err := png.Encode(&payload, imageData); err != nil {
		return model.TelegramQRLogin{}, err
	}

	expiresAt := token.Expires()
	return model.TelegramQRLogin{
		Status:        "pending",
		LoginURL:      token.URL(),
		QRCodeDataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(payload.Bytes()),
		ExpiresAt:     &expiresAt,
	}, nil
}

func telegramAccountFromUser(sessionFile string, user *tg.User) model.TelegramAccount {
	account := model.TelegramAccount{
		SessionFile: sessionFile,
		AuthState:   "authenticated",
	}
	if user != nil {
		account.Phone = user.Phone
	}
	return account
}
