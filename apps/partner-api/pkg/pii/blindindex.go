// Package pii — Fix-C item 6：BlindIndex(value, key) helper.
//
// Blind index = HMAC-SHA256(value, key) → hex(64 字符)。
// 用途：对加密的 PII（如 bank_account / legal_person_id）建立可搜索哈希索引；
// 与 cipher 列分离，DB 上索引仅泄漏哈希分布，不泄漏明文.
//
// Key 来源：BLIND_INDEX_KEY env var；与 DEK / KEK 派生槽位独立（per backend §3.9）。
//
// 调用方约定：value 必须先 normalize（trim / lower / 去分隔符）再传入；
// 否则同一账号的不同写法（"6228 4801 ..." vs "62284801..."）会落不同索引值.
package pii

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
)

// ErrBlindIndexKeyMissing BLIND_INDEX_KEY env var 未设置.
var ErrBlindIndexKeyMissing = errors.New("pii: BLIND_INDEX_KEY env var required for blind index HMAC")

// BlindIndex 返回 hex(HMAC-SHA256(key, value))；key 空 → 返错.
//
// 调用方负责传入归一化后的 value（参考 NormalizeBankAccount）.
func BlindIndex(value string, key []byte) (string, error) {
	if len(key) == 0 {
		return "", ErrBlindIndexKeyMissing
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// BlindIndexFromEnv 便捷封装：从 os.Getenv("BLIND_INDEX_KEY") 读取并 HMAC。
func BlindIndexFromEnv(value string) (string, error) {
	k := os.Getenv("BLIND_INDEX_KEY")
	if k == "" {
		return "", ErrBlindIndexKeyMissing
	}
	return BlindIndex(value, []byte(k))
}

// NormalizeBankAccount 移除空格 / 短横线 / 全角空格；保留数字字符.
//
// 严格归一化：HMAC 输入必须是 [0-9]+，否则同一账户多种写法会落不同 blind_index.
func NormalizeBankAccount(s string) string {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "　", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}
