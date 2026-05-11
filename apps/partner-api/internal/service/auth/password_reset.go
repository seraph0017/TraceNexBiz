// 双因子密码重置（PRD §17.5 + backend §7.9.1）。
//
// PR-INV-1..PR-INV-8 invariant 见 backend §7.9.2，本文件实现 1..5；
// invariant 6（consent）由 service.go 的 PasswordResetInitiate 入口校验；
// invariant 7（信息恒等）由 handler 层渲染恒等响应；
// invariant 8 由 cron `password_reset.purge` 兜底（W1c 实现）。
package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
)

// PasswordResetInitiate 阶段 1：发起重置（信息恒等响应；不泄漏命中）。
//
// PR-INV-6：必查 consent_log；PR-INV-7：handler 层把所有错误都转 200 文案。
func (s *Service) PasswordResetInitiate(ctx context.Context, in PasswordResetInitiateInput) error {
	cred, err := s.repo.FindCredentialsAny(ctx, in.ActorHandle)
	if err != nil {
		return nil
	}
	consented, err := s.consentRepo.HasConsent(ctx, cred.FyUserID, "password_reset", in.ConsentVersion)
	if err != nil || !consented {
		return ErrConsentRequired
	}
	rawToken, otp, err := s.generateResetMaterial()
	if err != nil {
		return fmt.Errorf("auth: gen reset material: %w", err)
	}
	now := s.clock()
	tok := PasswordResetToken{
		ActorType: cred.ActorType, ActorID: cred.ActorID,
		TokenHash:        hashHex(rawToken),
		SecondFactorType: "sms",
		SecondFactorHash: hashHex(otp),
		RequestedIP:      in.ClientIP,
		UserAgent:        in.UserAgent,
		ExpiresAt:        now.Add(s.resetTTL),
	}
	if err := s.repo.InsertResetToken(ctx, tok); err != nil {
		return fmt.Errorf("auth: insert reset token: %w", err)
	}
	if err := s.notifier.SendResetLink(ctx, cred.Email, rawToken); err != nil {
		return nil
	}
	if cred.Phone != "" {
		_ = s.notifier.SendResetOTP(ctx, cred.Phone, otp)
	}
	return nil
}

// PasswordResetConfirm 阶段 2：核 token + OTP；通过则改密 + 全设备 logout。
//
// 实现 PR-INV-1（TTL ≤ 15min）/ PR-INV-2（单次有效）/ PR-INV-3（5 次失败 invalidate）/
// PR-INV-4（成功后同步 revoke 全 jti）/ PR-INV-5（IP/UA 仅写库）。
func (s *Service) PasswordResetConfirm(ctx context.Context, in PasswordResetConfirmInput) error {
	tok, err := s.repo.FindResetTokenByHash(ctx, hashHex(in.RawToken))
	if err != nil {
		return ErrTokenInvalid
	}
	now := s.clock()
	if !tok.IsUsable(now) {
		return ErrTokenInvalid
	}
	if subtle.ConstantTimeCompare([]byte(tok.SecondFactorHash), []byte(hashHex(in.OTP))) != 1 {
		_ = s.repo.IncResetFailedAttempts(ctx, tok.ID, now)
		return ErrSecondFactorBad
	}
	newHash, err := s.hasher.Hash(in.NewPassword)
	if err != nil {
		return fmt.Errorf("auth: hash new pwd: %w", err)
	}
	if err := s.repo.ApplyPasswordReset(ctx, tok, newHash, now); err != nil {
		return fmt.Errorf("auth: apply reset: %w", err)
	}
	jtis, err := s.repo.ListActiveJTIs(ctx, tok.ActorType, tok.ActorID)
	if err != nil {
		return fmt.Errorf("auth: list jtis: %w", err)
	}
	for _, jti := range jtis {
		if err := s.revoke.Revoke(ctx, jti, s.refreshTTL); err != nil {
			return fmt.Errorf("auth: revoke %s: %w", jti, err)
		}
	}
	return s.repo.CloseAllSessions(ctx, tok.ActorType, tok.ActorID, now)
}
