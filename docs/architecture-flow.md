# 秒杀系统流程图

## 1. 整体架构流程

```mermaid
graph TB
    User[用户] --> Frontend[前端 Vue3]
    Frontend --> Nginx[Nginx]
    Nginx --> API[API服务 Gin]
    
    API --> Redis[(Redis)]
    API --> Kafka[Kafka消息队列]
    
    Kafka --> Worker[Worker消费者]
    Worker --> MySQL[(MySQL)]
    Worker --> Redis
    
    Worker --> RedisScanner[Redis超时扫描器]
    Worker --> MySQLScanner[MySQL兜底扫描器]
    
    RedisScanner --> MySQL
    MySQLScanner --> MySQL
    
    style Redis fill:#ff6b6b
    style Kafka fill:#4ecdc4
    style MySQL fill:#45b7d1
```

## 2. 秒杀核心流程（详细）

```mermaid
sequenceDiagram
    participant User as 用户
    participant API as API层
    participant Redis as Redis
    participant Kafka as Kafka
    participant Worker as Worker
    participant MySQL as MySQL
    
    User->>API: 1. POST /api/seckill/buy
    Note over User,API: T0: request_time
    
    API->>Redis: 2. 执行 Lua 脚本
    Note over Redis: seckill_check.lua
    
    Redis->>Redis: 2.1 检查限购数量
    Redis->>Redis: 2.2 遍历分段找库存
    Redis->>Redis: 2.3 扣减库存
    Redis->>Redis: 2.4 标记用户已购
    
    Redis-->>API: 3. 返回结果 (segmentID)
    Note over API,Redis: T1: create_time
    
    alt 库存充足
        API->>Kafka: 4. 发送消息
        Note over Kafka: T2: born_time
        API-->>User: 5. 返回"秒杀成功"
        
        Kafka->>Worker: 6. 批量拉取消息
        Note over Worker: 批量2000条/次
        Note over Kafka,Worker: T3: store_time
        
        Worker->>Redis: 7. 幂等性检查
        Note over Redis: seckill_decr.lua
        
        Worker->>MySQL: 8. 批量扣库存
        Worker->>MySQL: 9. 批量插入订单
        Note over Worker,MySQL: T4: write_time
        
        Worker->>Redis: 10. 添加到超时队列
        Note over Redis: ZSET + Hash
        
        Worker->>Kafka: 11. 提交 offset
        
    else 库存不足
        API-->>User: 返回"已售罄"
    else 超过限购
        API-->>User: 返回"超过限购"
    end
```

## 3. 库存分段流程

```mermaid
graph LR
    A[用户请求] --> B{随机选择起始分段}
    B --> C[分段0]
    B --> D[分段1]
    B --> E[分段2]
    B --> F[...]
    B --> G[分段31]
    
    C --> H{库存充足?}
    D --> H
    E --> H
    F --> H
    G --> H
    
    H -->|是| I[扣减库存]
    H -->|否| J[尝试下一分段]
    J --> H
    
    I --> K[返回 segmentID]
    
    style C fill:#ffe66d
    style D fill:#ffe66d
    style E fill:#ffe66d
    style G fill:#ffe66d
    style I fill:#4ecdc4
```

## 4. Lua 脚本原子操作

```mermaid
graph TB
    Start[开始] --> Check1{检查已购数量}
    Check1 -->|超限| Return1[返回 -1]
    Check1 -->|未超限| Loop[遍历32个分段]
    
    Loop --> Check2{当前分段库存充足?}
    Check2 -->|否| Next[下一分段]
    Next --> Check3{还有分段?}
    Check3 -->|是| Check2
    Check3 -->|否| Return2[返回 0 售罄]
    
    Check2 -->|是| Decr[扣减库存 DECRBY]
    Decr --> Mark[标记已购 HINCRBY]
    Mark --> Return3[返回 segmentID]
    
    style Check1 fill:#ff6b6b
    style Check2 fill:#ff6b6b
    style Decr fill:#4ecdc4
    style Mark fill:#4ecdc4
```

## 5. 订单超时取消流程

```mermaid
graph TB
    A[订单创建] --> B[添加到 Redis ZSET]
    B --> C{Redis 扫描器<br/>500ms间隔}
    
    C --> D{订单过期?}
    D -->|否| C
    D -->|是| E[Lua 原子弹出]
    
    E --> F[批量取消订单]
    F --> G[更新 MySQL status=2]
    G --> H[返还 MySQL 库存]
    H --> I[返还 Redis 库存]
    I --> J[清除用户标记]
    
    B --> K{MySQL 扫描器<br/>5分钟间隔}
    K --> L{查询超时订单}
    L --> M[兜底取消]
    M --> G
    
    style C fill:#ffe66d
    style K fill:#a8dadc
    style E fill:#ff6b6b
    style F fill:#ff6b6b
```

## 6. 批量处理流程

```mermaid
graph LR
    A[Kafka 消息] --> B[批量拉取<br/>2000条/次]
    B --> C[按商品ID分组]
    
    C --> D1[商品1订单]
    C --> D2[商品2订单]
    C --> D3[商品N订单]
    
    D1 --> E1[批量幂等检查]
    D2 --> E2[批量幂等检查]
    D3 --> E3[批量幂等检查]
    
    E1 --> F1[批量写入<br/>1000条/批]
    E2 --> F2[批量写入<br/>1000条/批]
    E3 --> F3[批量写入<br/>1000条/批]
    
    F1 --> G[提交 offset]
    F2 --> G
    F3 --> G
    
    style B fill:#4ecdc4
    style C fill:#ffe66d
    style F1 fill:#45b7d1
    style F2 fill:#45b7d1
    style F3 fill:#45b7d1
```

## 7. 库存预热流程

```mermaid
graph TB
    Start[开始预热] --> Lock{获取分布式锁}
    Lock -->|失败| End1[返回预热中]
    Lock -->|成功| Clear[清理旧数据]
    
    Clear --> Query[查询 MySQL 库存]
    Query --> Calc[计算分段库存]
    
    Calc --> Set1[分段0: baseStock + 1]
    Calc --> Set2[分段1: baseStock + 1]
    Calc --> Set3[分段2: baseStock]
    Calc --> Set4[...]
    Calc --> Set5[分段31: baseStock]
    
    Set1 --> Release[释放锁]
    Set2 --> Release
    Set3 --> Release
    Set4 --> Release
    Set5 --> Release
    
    Release --> End2[预热完成]
    
    style Lock fill:#ff6b6b
    style Calc fill:#4ecdc4
    style Set1 fill:#ffe66d
    style Set2 fill:#ffe66d
    style Set3 fill:#ffe66d
    style Set5 fill:#ffe66d
```

## 8. 幂等性保证流程

```mermaid
graph TB
    A[Worker 收到消息] --> B[解析消息]
    B --> C{Redis 检查<br/>已购记录}
    
    C -->|不存在| D[异常: 发送到 DLQ]
    C -->|存在| E{Redis 检查<br/>已处理记录}
    
    E -->|已处理| F[跳过: 重复消费]
    E -->|未处理| G[标记已处理]
    
    G --> H[MySQL 写入订单]
    H --> I{主键冲突?}
    
    I -->|是| J[跳过: 已存在]
    I -->|否| K[写入成功]
    
    K --> L[提交 offset]
    F --> L
    J --> L
    
    style C fill:#ffe66d
    style E fill:#ffe66d
    style G fill:#4ecdc4
    style I fill:#ff6b6b
```

## 9. 容错处理流程

```mermaid
graph TB
    A[API 层] --> B{Redis 扣库存}
    B -->|成功| C{Kafka 发送}
    B -->|失败| D[返回错误]
    
    C -->|成功| E[返回成功]
    C -->|失败| F[返还 Redis 库存]
    F --> G[清除用户标记]
    G --> D
    
    H[Worker 层] --> I{MySQL 写入}
    I -->|成功| J[提交 offset]
    I -->|库存不足| K[返还 Redis 库存]
    K --> L[清除用户标记]
    L --> M[发送到 DLQ]
    
    I -->|主键冲突| N[幂等: 跳过]
    N --> J
    
    I -->|其他错误| O[清除已处理标记]
    O --> M
    
    style B fill:#ffe66d
    style C fill:#ffe66d
    style I fill:#ffe66d
    style F fill:#ff6b6b
    style K fill:#ff6b6b
```

## 10. 完整数据流

```mermaid
graph TB
    subgraph "用户层"
        U[用户浏览器]
    end
    
    subgraph "接入层"
        N[Nginx]
    end
    
    subgraph "应用层"
        A[API 服务]
        W[Worker 服务]
        RS[Redis 扫描器]
        MS[MySQL 扫描器]
    end
    
    subgraph "缓存层"
        R1[Redis 分段库存]
        R2[Redis 限购记录]
        R3[Redis 超时队列]
    end
    
    subgraph "消息层"
        K[Kafka 64分区]
    end
    
    subgraph "存储层"
        M1[MySQL 商品表]
        M2[MySQL 订单表]
    end
    
    U --> N
    N --> A
    
    A --> R1
    A --> R2
    A --> K
    
    K --> W
    W --> R2
    W --> R3
    W --> M1
    W --> M2
    
    RS --> R3
    RS --> M1
    RS --> M2
    
    MS --> M2
    MS --> M1
    
    style R1 fill:#ff6b6b
    style R2 fill:#ff6b6b
    style R3 fill:#ff6b6b
    style K fill:#4ecdc4
    style M1 fill:#45b7d1
    style M2 fill:#45b7d1
```

## 时间线说明

| 时间点 | 名称 | 说明 | 典型耗时 |
|--------|------|------|----------|
| T0 | request_time | 用户点击秒杀 | 0ms |
| T1 | create_time | Redis 确认成功 | ~30ms |
| T2 | born_time | Kafka Producer 发送 | ~35ms |
| T3 | store_time | Kafka Broker 存储 | ~40ms |
| T4 | write_time | MySQL 写入完成 | ~80ms |

**订单落库 延迟**: T4 - T0 ≈ 80ms (P50)
