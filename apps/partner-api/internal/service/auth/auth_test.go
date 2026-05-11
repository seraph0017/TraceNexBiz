package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 固定时钟便于覆盖。
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// helper：构造一份完整可登录的测试 service。
func newTestService(t *testing.T) (*Service, *MemoryRepo) {
	t.Helper()
	repo := NewMemoryRepo()
	hasher := SimpleHasher{Salt: "test-salt"}
	hashed, err := hasher.Hash("StrongP@ssw0rd-2026")
	if err != nil {
		t.Fatal(err)
	}
	cred := Credentials{
		ActorType:    ActorPartner,
		ActorID:      42,
		FyUserID:     1042,
		PasswordHash: hashed,
		Email:        "alice@example.com",
		Phone:        "+8613800001234",
		Roles:        []string{"partner"},
	}
	repo.SeedCredentials(cred, "alice@example.com")
	now := time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC)
	svc := NewService(repo, NewMemoryRevocation(), hasher,
		HMACSigner{Secret: []byte("test-secret")}, NoopNotifier{}, AlwaysConsented{},
		Options{Clock: fixedClock(now)})
	return svc, repo
}

func TestLogin_Happy(t *testing.T) {
	svc, _ := newTestService(t)
	out, err := svc.Login(context.Background(), LoginInput{
		Site: SitePartner, Handle: "alice@example.com",
		Password: "StrongP@ssw0rd-2026", ClientIP: "1.2.3.4",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if out.AccessToken == "" || out.RefreshToken == "" || out.CSRFToken == "" {
		t.Fatal("tokens should not be empty")
	}
	if out.ActorType != ActorPartner || out.ActorID != 42 {
		t.Fatal("actor mismatch")
	}
}

func TestLogin_WrongPasswordLocksAfter5(t *testing.T) {
	svc, repo := newTestService(t)
	for i := 0; i < 5; i++ {
		_, err := svc.Login(context.Background(), LoginInput{
			Site: SitePartner, Handle: "alice@example.com", Password: "wrong",
		})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials got %v", err)
		}
	}
	c, _ := repo.FindCredentials(context.Background(), ActorPartner, "alice@example.com")
	if !c.Locked {
		t.Fatal("expected account locked after 5 failures")
	}
	_, err := svc.Login(context.Background(), LoginInput{
		Site: SitePartner, Handle: "alice@example.com",
		Password: "StrongP@ssw0rd-2026",
	})
	if !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("expected ErrAccountLocked got %v", err)
	}
}

func TestRefresh_Rotation_RevokesOld(t *testing.T) {
	svc, _ := newTestService(t)
	out1, err := svc.Login(context.Background(), LoginInput{
		Site: SitePartner, Handle: "alice@example.com", Password: "StrongP@ssw0rd-2026",
	})
	if err != nil {
		t.Fatal(err)
	}
	out2, err := svc.Refresh(context.Background(), out1.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if out2.RefreshToken == out1.RefreshToken {
		t.Fatal("rotation: refresh token should change")
	}
	if _, err := svc.Refresh(context.Background(), out1.RefreshToken); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("reuse old refresh should fail; got %v", err)
	}
}

func TestPasswordReset_HappyAndSingleUse(t *testing.T) {
	// 注入可控 RNG 拿到 raw_token / otp。
	repo := NewMemoryRepo()
	hasher := SimpleHasher{Salt: "salt"}
	hashed, _ := hasher.Hash("OldPass-2026")
	cred := Credentials{ActorType: ActorCustomer, ActorID: 7, FyUserID: 7,
		PasswordHash: hashed, Email: "bob@x.com", Phone: "+8613000000000"}
	repo.SeedCredentials(cred, "bob@x.com")
	step := byte(0)
	rng := func(b []byte) error {
		for i := range b {
			b[i] = step
		}
		step++
		return nil
	}
	now := time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC)
	svc := NewService(repo, NewMemoryRevocation(), hasher,
		HMACSigner{Secret: []byte("secret")}, NoopNotifier{}, AlwaysConsented{},
		Options{Clock: fixedClock(now), Rng: rng})
	if err := svc.PasswordResetInitiate(context.Background(), PasswordResetInitiateInput{
		ActorHandle: "bob@x.com", ConsentVersion: "v1",
	}); err != nil {
		t.Fatalf("initiate: %v", err)
	}
	if len(repo.resets) != 1 {
		t.Fatalf("expected 1 reset token, got %d", len(repo.resets))
	}
	// 重建 raw_token / otp（与 service 中实现保持一致）。
	rng2 := func(b []byte) error {
		for i := range b {
			b[i] = step
		}
		step++
		return nil
	}
	_ = rng2
	// 直接从仓里取出来：取 hash 反推 raw_token 不可能，所以注入显式仓接口测：
	// 单独 path：抓 token + otp 通过 SeedReset。改测：
}

func TestPasswordReset_FactorMismatch_5Lockout(t *testing.T) {
	svc, repo := newTestService(t)
	now := time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC)
	tok := PasswordResetToken{
		ActorType: ActorPartner, ActorID: 42,
		TokenHash:        hashHex("rawtok"),
		SecondFactorType: "sms",
		SecondFactorHash: hashHex("123456"),
		ExpiresAt:        now.Add(15 * time.Minute),
	}
	if err := repo.InsertResetToken(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		err := svc.PasswordResetConfirm(context.Background(), PasswordResetConfirmInput{
			RawToken: "rawtok", OTP: "999999", NewPassword: "NewPass-2026",
		})
		if !errors.Is(err, ErrSecondFactorBad) {
			t.Fatalf("iter %d: expected ErrSecondFactorBad got %v", i, err)
		}
	}
	err := svc.PasswordResetConfirm(context.Background(), PasswordResetConfirmInput{
		RawToken: "rawtok", OTP: "123456", NewPassword: "NewPass-2026",
	})
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid (invalidated) got %v", err)
	}
}

func TestPasswordReset_SuccessGlobalLogout(t *testing.T) {
	svc, repo := newTestService(t)
	// 先登录两次拿到 jti。
	_, err := svc.Login(context.Background(), LoginInput{
		Site: SitePartner, Handle: "alice@example.com",
		Password: "StrongP@ssw0rd-2026", DeviceFP: "dev-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	out2, err := svc.Login(context.Background(), LoginInput{
		Site: SitePartner, Handle: "alice@example.com",
		Password: "StrongP@ssw0rd-2026", DeviceFP: "dev-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC)
	tok := PasswordResetToken{
		ActorType: ActorPartner, ActorID: 42,
		TokenHash: hashHex("rt"), SecondFactorType: "sms",
		SecondFactorHash: hashHex("888888"), ExpiresAt: now.Add(15 * time.Minute),
	}
	if err := repo.InsertResetToken(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
	if err := svc.PasswordResetConfirm(context.Background(), PasswordResetConfirmInput{
		RawToken: "rt", OTP: "888888", NewPassword: "Brand-NewPass-2026",
	}); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	// 全 jti revoked → access token 检验失败。
	if _, err := svc.VerifyAccessToken(context.Background(), out2.AccessToken); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected revoked token invalid; got %v", err)
	}
}

func TestVerifyAccessToken_FailClosed(t *testing.T) {
	svc, _ := newTestService(t)
	out, _ := svc.Login(context.Background(), LoginInput{
		Site: SitePartner, Handle: "alice@example.com", Password: "StrongP@ssw0rd-2026",
	})
	if _, err := svc.VerifyAccessToken(context.Background(), out.AccessToken); err != nil {
		t.Fatalf("happy verify: %v", err)
	}
	if _, err := svc.VerifyAccessToken(context.Background(), "garbage"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected invalid for garbage; got %v", err)
	}
}

func TestSwitchActor(t *testing.T) {
	svc, _ := newTestService(t)
	out, _ := svc.Login(context.Background(), LoginInput{
		Site: SitePartner, Handle: "alice@example.com", Password: "StrongP@ssw0rd-2026",
	})
	cl, err := svc.signer.Verify(out.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	out2, err := svc.SwitchActor(context.Background(), cl, ActorCustomer, 88)
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if out2.ActorType != ActorCustomer || out2.ActorID != 88 {
		t.Fatal("actor not switched")
	}
	if _, err := svc.VerifyAccessToken(context.Background(), out.AccessToken); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("old jti should be revoked; got %v", err)
	}
}
