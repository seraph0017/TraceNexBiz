-- 017_red_flush_status_enum.up.sql — DEFERRED (TODO Fix-C item 9)
--
-- PRD §invoice 要求 red_flush_request.status 枚举：
--   (pending, approved, dispatched, completed, rejected)
--
-- 当前实现（migration 005 + invoice service）使用：
--   (applied, awaiting_tax_confirm, confirmed, completed, rejected)
--
-- 这两个集合值集合不同；切换需要：
--   1) invoice service / red_flush state machine 重写（issued → red_flushing → red_flushed）
--   2) frontend 工单状态映射表更新
--   3) 历史 red_flush_request 行迁移（applied→pending, awaiting_tax_confirm→approved,
--      confirmed→dispatched）
--
-- Fix-C 范围内仅记录 enum drift；实际重写在 Fix-D 后由 invoice agent 单独执行.
-- 本文件保持 no-op，避免触发 invoice 服务路径回归.

SELECT 1;
