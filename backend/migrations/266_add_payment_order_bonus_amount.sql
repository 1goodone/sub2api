-- 充值优惠中的免费额度：余额充值订单命中赠送或折扣阶梯时的免费 USD 金额。
-- 已计入 payment_orders.amount（到账总额），单独落列用于订单展示与推广返利基数剔除。
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS bonus_amount DECIMAL(20,2) NOT NULL DEFAULT 0;
