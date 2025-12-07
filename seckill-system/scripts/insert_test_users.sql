-- 批量插入 10000 个测试账号
-- 用户名: test0001 ~ test10000
-- 邮箱: test0001@test.com ~ test10000@test.com
-- 密码: 123456（bcrypt 加密后）
-- 昵称: 测试用户0001 ~ 测试用户10000

-- 先删除已存在的测试账号（可选）
-- DELETE FROM users WHERE username LIKE 'test%';

-- 使用存储过程批量插入
DELIMITER //

DROP PROCEDURE IF EXISTS insert_test_users//

CREATE PROCEDURE insert_test_users()
BEGIN
    DECLARE i INT DEFAULT 1;
    DECLARE batch_size INT DEFAULT 1000;
    DECLARE username VARCHAR(50);
    DECLARE email VARCHAR(100);
    DECLARE nickname VARCHAR(100);
    -- bcrypt 加密的 "123456" (cost=10)
    -- 使用 Go 生成: bcrypt.GenerateFromPassword([]byte("123456"), 10)
    DECLARE pwd VARCHAR(255) DEFAULT '$2a$10$EixZaYVK1fsbw1ZfbX3OXePaWxn96p36WQoeG6Lruj3vjPGga31lW';
    
    -- 关闭自动提交，提高插入性能
    SET autocommit = 0;
    
    WHILE i <= 10000 DO
        SET username = CONCAT('test', LPAD(i, 5, '0'));
        SET email = CONCAT('test', LPAD(i, 5, '0'), '@test.com');
        SET nickname = CONCAT('测试用户', LPAD(i, 5, '0'));
        
        -- 插入用户，忽略重复
        INSERT IGNORE INTO users (username, password, email, nickname, created_at, updated_at)
        VALUES (username, pwd, email, nickname, NOW(), NOW());
        
        -- 每 1000 条提交一次
        IF i MOD batch_size = 0 THEN
            COMMIT;
            SELECT CONCAT('已插入 ', i, ' 条记录') AS progress;
        END IF;
        
        SET i = i + 1;
    END WHILE;
    
    -- 最后提交
    COMMIT;
    SET autocommit = 1;
    
    SELECT '插入完成！共 10000 个测试账号' AS result;
END//

DELIMITER ;

-- 执行存储过程
CALL insert_test_users();

-- 删除存储过程
DROP PROCEDURE IF EXISTS insert_test_users;

-- 验证插入结果
SELECT COUNT(*) AS total_test_users FROM users WHERE username LIKE 'test%';
