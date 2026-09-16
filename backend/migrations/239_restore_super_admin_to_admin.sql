-- 运营增强 246_account_protection_roles.sql 会把最早的 active admin 升成 super_admin。
-- 该功能已从代码回退：鉴权与侧边栏只认 role='admin'，残留 super_admin 会让
-- 「账号管理」等管理入口消失。把 super_admin 改回 admin。
-- 幂等：无 super_admin 时影响 0 行。

UPDATE users
SET role = 'admin',
    updated_at = NOW()
WHERE role = 'super_admin';
