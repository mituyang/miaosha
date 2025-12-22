CREATE DATABASE IF NOT EXISTS seckill DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE seckill;

-- 商品表 (商品数量少，可以用自增，也可以改成分布式ID，这里为了后台管理方便保留自增)
CREATE TABLE IF NOT EXISTS goods (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '商品ID',
    product_name VARCHAR(255) NOT NULL COMMENT '商品名称',
    stock INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '库存数量',
    price DECIMAL(10, 2) NOT NULL DEFAULT 0.00 COMMENT '秒杀价格',
    version INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '乐观锁版本号',
    PRIMARY KEY (id),
    CONSTRAINT chk_stock_non_negative CHECK (stock >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='商品表';

-- 订单表 (核心修改)
CREATE TABLE IF NOT EXISTS orders (
    -- 【修改点1】移除了 AUTO_INCREMENT
    -- 【修改点2】id 即为雪花算法生成的订单号
    id BIGINT UNSIGNED NOT NULL COMMENT '订单ID(雪花算法生成)',
    
    user_id BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
    goods_id BIGINT UNSIGNED NOT NULL COMMENT '商品ID',
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
    INDEX idx_goods_id (goods_id) -- 库存返还查询优化
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='订单表';

-- users表
CREATE TABLE `users` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `username` varchar(50) NOT NULL COMMENT '用户名',
  `password` varchar(255) NOT NULL COMMENT '密码哈希',
  `created_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表';


-- 插入测试数据
INSERT INTO goods (product_name, stock, price, version) VALUES 
('iPhone 15 Pro', 10000, 6999.00, 0),
('MacBook Pro M3', 50, 12999.00, 0);