CREATE DATABASE IF NOT EXISTS seckill DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE seckill;

-- 商品表 (商品数量少，可以用自增，也可以改成分布式ID，这里为了后台管理方便保留自增)
CREATE TABLE IF NOT EXISTS goods (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '商品ID',
    product_name VARCHAR(255) NOT NULL COMMENT '商品名称',
    description VARCHAR(500) NOT NULL DEFAULT '' COMMENT '商品描述',
    stock INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '库存数量',
    price DECIMAL(10, 2) NOT NULL DEFAULT 0.00 COMMENT '秒杀价格',
    status TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '商品状态: 0-下架, 1-上架',
    version INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '乐观锁版本号',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    PRIMARY KEY (id),
    KEY idx_goods_status (status),
    CONSTRAINT chk_stock_non_negative CHECK (stock >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='商品表';

-- 秒杀活动表
CREATE TABLE IF NOT EXISTS seckill_activities (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '活动ID',
    goods_id BIGINT UNSIGNED NOT NULL COMMENT '商品ID',
    title VARCHAR(255) NOT NULL COMMENT '活动标题',
    start_time DATETIME(3) NOT NULL COMMENT '开始时间',
    end_time DATETIME(3) NOT NULL COMMENT '结束时间',
    status TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '活动状态: 0-未开始, 1-进行中, 2-已结束, 3-停用',
    max_buy_limit INT UNSIGNED NOT NULL COMMENT '每用户最大购买数量',
    warmup_status TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '预热状态: 0-未预热, 1-已预热',
    is_default TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '是否默认活动: 0-否, 1-是',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    PRIMARY KEY (id),
    KEY idx_activity_goods_id (goods_id),
    KEY idx_activity_goods_default (goods_id, is_default),
    KEY idx_activity_status_time (status, start_time, end_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='秒杀活动表';

-- 管理员表
CREATE TABLE IF NOT EXISTS admin_users (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '管理员ID',
    username VARCHAR(50) NOT NULL COMMENT '管理员账号',
    password VARCHAR(255) NOT NULL COMMENT '密码哈希',
    status TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '管理员状态: 0-禁用, 1-启用',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_admin_username (username),
    KEY idx_admin_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='管理员表';

-- 订单表 (核心修改)
CREATE TABLE IF NOT EXISTS orders (
    -- 【修改点1】移除了 AUTO_INCREMENT
    -- 【修改点2】id 即为雪花算法生成的订单号
    id BIGINT UNSIGNED NOT NULL COMMENT '订单ID(雪花算法生成)',
    
    user_id BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
    goods_id BIGINT UNSIGNED NOT NULL COMMENT '商品ID',
    activity_id BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '秒杀活动ID',
    quantity INT UNSIGNED NOT NULL DEFAULT 1 COMMENT '购买数量',
    pay_amount DECIMAL(10, 2) NOT NULL DEFAULT 0.00 COMMENT '支付金额',
    status TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '订单状态: 0-未支付, 1-已支付, 2-已取消',
    request_time DATETIME(3) NOT NULL COMMENT '用户请求时间',
    create_time DATETIME(3) NOT NULL COMMENT 'Redis确认时间(订单创建时间)',
    born_time DATETIME(3) NOT NULL COMMENT 'MQ Producer发送时间',
    store_time DATETIME(3) NOT NULL COMMENT 'MQ Broker存储时间',
    write_time DATETIME(3) NOT NULL COMMENT 'MySQL写入时间',
    pay_time DATETIME(3) DEFAULT NULL COMMENT '支付时间',
    cancel_time DATETIME(3) DEFAULT NULL COMMENT '取消时间',
    
    PRIMARY KEY (id), -- 分布式ID作为主键
    INDEX idx_user_id (user_id),
    INDEX idx_user_goods_status (user_id, goods_id, status),
    INDEX idx_status_write_time (status, write_time), -- 超时订单扫描优化
    INDEX idx_activity_id (activity_id),
    INDEX idx_orders_create_time (create_time),
    INDEX idx_orders_status_create_time (status, create_time),
    INDEX idx_goods_id (goods_id) -- 库存返还查询优化
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='订单表';

-- users表
CREATE TABLE `users` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `username` varchar(50) NOT NULL COMMENT '用户名',
  `email` varchar(255) NOT NULL DEFAULT '' COMMENT '邮箱',
  `password` varchar(255) NOT NULL COMMENT '密码哈希',
  `status` tinyint unsigned NOT NULL DEFAULT 1 COMMENT '用户状态: 0-禁用, 1-启用',
  `created_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`),
  KEY `idx_users_email` (`email`),
  KEY `idx_users_status` (`status`),
  KEY `idx_users_created_at` (`created_at`),
  KEY `idx_users_status_created_at` (`status`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表';


-- 插入测试数据
INSERT INTO goods (product_name, description, stock, price, status, version) VALUES 
('iPhone 15 Pro', 'A17 Pro 芯片，适用于高并发秒杀演示', 10000, 6999.00, 1, 0),
('MacBook Pro M3', '高性能笔记本，适用于高客单价商品演示', 50, 12999.00, 1, 0);
