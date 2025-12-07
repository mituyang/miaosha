-- 创建数据库
CREATE DATABASE IF NOT EXISTS seckill DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE seckill;

-- 秒杀商品表
CREATE TABLE IF NOT EXISTS seckill_goods (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    goods_name VARCHAR(255) NOT NULL COMMENT '商品名称',
    stock INT NOT NULL DEFAULT 0 COMMENT '库存数量',
    start_time DATETIME NOT NULL COMMENT '秒杀开始时间',
    end_time DATETIME NOT NULL COMMENT '秒杀结束时间',
    version INT NOT NULL DEFAULT 0 COMMENT '乐观锁版本号',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_start_time (start_time),
    INDEX idx_end_time (end_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='秒杀商品表';

-- 秒杀订单表
CREATE TABLE IF NOT EXISTS seckill_order (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL COMMENT '订单号',
    user_id VARCHAR(50) NOT NULL COMMENT '用户名',
    goods_id BIGINT NOT NULL COMMENT '商品ID',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '订单状态: 0-待支付, 1-已支付, 2-已取消',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_order_id (order_id),
    INDEX idx_user_id (user_id),
    INDEX idx_goods_id (goods_id),
    INDEX idx_user_goods_status (user_id, goods_id, status) COMMENT '用于查询用户商品的有效订单'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='秒杀订单表';

-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(50) NOT NULL COMMENT '用户名',
    password VARCHAR(255) NOT NULL COMMENT '密码（加密）',
    email VARCHAR(100) NOT NULL COMMENT '邮箱',
    nickname VARCHAR(100) COMMENT '昵称',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_username (username),
    UNIQUE INDEX uk_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表';

-- 插入测试数据
INSERT INTO seckill_goods (goods_name, stock, start_time, end_time) VALUES
('iPhone 15 Pro 秒杀专场', 100, '2024-01-01 10:00:00', '2024-12-31 23:59:59'),
('MacBook Pro M3 限时抢购', 50, '2024-01-01 10:00:00', '2024-12-31 23:59:59');
