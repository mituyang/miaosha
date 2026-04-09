<template>
  <div class="admin-page">
    <div v-if="!verified" class="admin-auth-shell">
      <section class="admin-auth-card">
        <div class="auth-copy">
          <span class="admin-badge">Admin</span>
          <h1 class="auth-title">后台管理</h1>
          <p class="auth-desc">这是独立于秒杀前台的管理站点入口，输入管理密钥后进入后台。</p>
        </div>

        <div v-if="authError" class="alert alert-error">{{ authError }}</div>

        <form class="auth-form" @submit.prevent="handleAdminLogin">
          <div class="form-group">
            <label class="form-label" for="admin-secret">管理密钥</label>
            <input
              id="admin-secret"
              v-model="secretInput"
              type="password"
              class="form-input"
              placeholder="请输入 X-Admin-Secret"
              autocomplete="current-password"
            />
          </div>
          <button type="submit" class="btn btn-primary btn-block" :disabled="authLoading">
            {{ authLoading ? '验证中...' : '进入后台' }}
          </button>
        </form>
      </section>
    </div>

    <div v-else class="admin-shell">
      <header class="admin-header">
        <div>
          <p class="admin-eyebrow">Operations</p>
          <h1 class="admin-title">秒杀后台管理</h1>
        </div>
        <div class="admin-header-actions">
          <button class="btn btn-secondary" @click="refreshCurrentTab" :disabled="tabLoading">
            {{ tabLoading ? '刷新中...' : '刷新当前模块' }}
          </button>
          <a href="/" class="btn btn-secondary admin-link">前往秒杀前台</a>
          <button class="btn btn-secondary" @click="logoutAdmin">退出后台</button>
        </div>
      </header>

      <nav class="admin-tabs" aria-label="后台模块">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          :class="['tab-button', { active: activeTab === tab.key }]"
          @click="activeTab = tab.key"
        >
          {{ tab.label }}
        </button>
      </nav>

      <section v-if="activeTab === 'goods'" class="admin-section">
        <div class="panel">
          <div class="panel-header">
            <h2>商品管理</h2>
            <button class="btn btn-primary" @click="prepareCreateGoods">新增商品</button>
          </div>

          <div class="toolbar">
            <input v-model="goodsFilters.keyword" class="form-input toolbar-input" placeholder="搜索商品名称" />
            <select v-model="goodsFilters.status" class="form-input toolbar-select">
              <option value="">全部状态</option>
              <option value="1">上架</option>
              <option value="0">下架</option>
            </select>
            <button class="btn btn-secondary" @click="handleGoodsSearch">查询</button>
            <button class="btn btn-secondary" @click="resetGoodsFilters">重置</button>
          </div>

          <form class="goods-form-card" @submit.prevent="submitGoodsForm">
            <div class="panel-header slim">
              <h3>{{ goodsForm.id ? '编辑商品' : '新增商品' }}</h3>
              <button v-if="goodsForm.id" type="button" class="btn btn-secondary" @click="resetGoodsForm">取消编辑</button>
            </div>
            <div class="goods-form-grid">
              <div class="form-group">
                <label class="form-label">商品名称</label>
                <input v-model="goodsForm.productName" class="form-input" maxlength="255" placeholder="请输入商品名称" />
              </div>
              <div class="form-group">
                <label class="form-label">价格</label>
                <input v-model.number="goodsForm.price" type="number" min="0" step="0.01" class="form-input" placeholder="0.00" />
              </div>
              <div class="form-group">
                <label class="form-label">库存</label>
                <input v-model.number="goodsForm.stock" type="number" min="0" step="1" class="form-input" placeholder="0" />
              </div>
              <div class="form-group">
                <label class="form-label">状态</label>
                <select v-model.number="goodsForm.status" class="form-input">
                  <option :value="1">上架</option>
                  <option :value="0">下架</option>
                </select>
              </div>
              <div class="form-group form-group-full">
                <label class="form-label">商品描述</label>
                <textarea v-model="goodsForm.description" class="form-input form-textarea" maxlength="500" placeholder="请输入商品简介"></textarea>
              </div>
            </div>
            <button type="submit" class="btn btn-primary" :disabled="savingGoods">
              {{ savingGoods ? '保存中...' : (goodsForm.id ? '保存修改' : '创建商品') }}
            </button>
          </form>

          <div class="table-shell">
            <table class="admin-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>商品</th>
                  <th>价格</th>
                  <th>库存</th>
                  <th>状态</th>
                  <th>更新时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="goods in goodsList" :key="goods.id">
                  <td>{{ goods.id }}</td>
                  <td>
                    <div class="table-title">{{ goods.productName }}</div>
                    <div class="table-subtitle">{{ goods.description || '暂无描述' }}</div>
                  </td>
                  <td>{{ formatCurrency(goods.price) }}</td>
                  <td>{{ goods.stock }}</td>
                  <td><span :class="['status-badge', goods.status === 1 ? 'status-on' : 'status-off']">{{ goodsStatusText(goods.status) }}</span></td>
                  <td>{{ formatTime(goods.updatedAt) }}</td>
                  <td>
                    <div class="action-row">
                      <button class="text-button" @click="editGoods(goods)">编辑</button>
                      <button class="text-button" @click="handleWarmUpSingle(goods.id)">预热</button>
                      <button class="text-button danger" @click="removeGoods(goods)">删除</button>
                    </div>
                  </td>
                </tr>
                <tr v-if="goodsList.length === 0">
                  <td colspan="7" class="table-empty">暂无商品数据</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="pager" v-if="pagination.goods.total > 0">
            <span>第 {{ pagination.goods.page }} / {{ totalPages('goods') }} 页，共 {{ pagination.goods.total }} 条</span>
            <div class="pager-actions">
              <button class="btn btn-secondary" :disabled="pagination.goods.page <= 1 || loadingState.goods" @click="changePage('goods', pagination.goods.page - 1)">上一页</button>
              <button class="btn btn-secondary" :disabled="pagination.goods.page >= totalPages('goods') || loadingState.goods" @click="changePage('goods', pagination.goods.page + 1)">下一页</button>
            </div>
          </div>
        </div>
      </section>

      <section v-if="activeTab === 'orders'" class="admin-section">
        <div class="panel">
          <div class="panel-header">
            <h2>订单管理</h2>
          </div>

          <div class="toolbar">
            <input v-model="orderFilters.keyword" class="form-input toolbar-input" placeholder="搜索订单号、用户或商品" />
            <select v-model="orderFilters.status" class="form-input toolbar-select">
              <option value="">全部状态</option>
              <option value="0">待支付</option>
              <option value="1">已支付</option>
              <option value="2">已取消</option>
            </select>
            <button class="btn btn-secondary" @click="handleOrderSearch">查询</button>
            <button class="btn btn-secondary" @click="resetOrderFilters">重置</button>
          </div>

          <div class="table-shell">
            <table class="admin-table">
              <thead>
                <tr>
                  <th>订单号</th>
                  <th>用户</th>
                  <th>商品</th>
                  <th>数量</th>
                  <th>金额</th>
                  <th>状态</th>
                  <th>下单时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="order in orders" :key="order.id">
                  <td>{{ order.id }}</td>
                  <td>{{ order.username }}</td>
                  <td>{{ order.goodsName }}</td>
                  <td>{{ order.quantity }}</td>
                  <td>{{ formatCurrency(order.payAmount) }}</td>
                  <td><span :class="['status-badge', orderStatusClass(order.status)]">{{ orderStatusText(order.status) }}</span></td>
                  <td>{{ formatTime(order.createTime) }}</td>
                  <td><button class="text-button" @click="fetchOrderDetail(order.id)">详情</button></td>
                </tr>
                <tr v-if="orders.length === 0">
                  <td colspan="8" class="table-empty">暂无订单数据</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="pager" v-if="pagination.orders.total > 0">
            <span>第 {{ pagination.orders.page }} / {{ totalPages('orders') }} 页，共 {{ pagination.orders.total }} 条</span>
            <div class="pager-actions">
              <button class="btn btn-secondary" :disabled="pagination.orders.page <= 1 || loadingState.orders" @click="changePage('orders', pagination.orders.page - 1)">上一页</button>
              <button class="btn btn-secondary" :disabled="pagination.orders.page >= totalPages('orders') || loadingState.orders" @click="changePage('orders', pagination.orders.page + 1)">下一页</button>
            </div>
          </div>

          <div v-if="selectedOrder" class="detail-card">
            <div class="panel-header slim">
              <h3>订单详情</h3>
              <button class="btn btn-secondary" @click="selectedOrder = null">关闭</button>
            </div>
            <div class="detail-grid">
              <div><span>订单号</span><strong>{{ selectedOrder.id }}</strong></div>
              <div><span>用户</span><strong>{{ selectedOrder.username }}</strong></div>
              <div><span>商品</span><strong>{{ selectedOrder.goodsName }}</strong></div>
              <div><span>状态</span><strong>{{ orderStatusText(selectedOrder.status) }}</strong></div>
              <div><span>数量</span><strong>{{ selectedOrder.quantity }}</strong></div>
              <div><span>金额</span><strong>{{ formatCurrency(selectedOrder.payAmount) }}</strong></div>
              <div><span>请求时间</span><strong>{{ formatTime(selectedOrder.requestTime) }}</strong></div>
              <div><span>创建时间</span><strong>{{ formatTime(selectedOrder.createTime) }}</strong></div>
              <div><span>支付时间</span><strong>{{ formatTime(selectedOrder.payTime) }}</strong></div>
              <div><span>取消时间</span><strong>{{ formatTime(selectedOrder.cancelTime) }}</strong></div>
            </div>
          </div>
        </div>
      </section>

      <section v-if="activeTab === 'users'" class="admin-section">
        <div class="panel">
          <div class="panel-header">
            <h2>用户管理</h2>
          </div>

          <div class="toolbar">
            <input v-model="userFilters.keyword" class="form-input toolbar-input" placeholder="搜索用户名" />
            <select v-model="userFilters.status" class="form-input toolbar-select">
              <option value="">全部状态</option>
              <option value="1">启用</option>
              <option value="0">禁用</option>
            </select>
            <button class="btn btn-secondary" @click="handleUserSearch">查询</button>
            <button class="btn btn-secondary" @click="resetUserFilters">重置</button>
          </div>

          <div class="table-shell">
            <table class="admin-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>用户名</th>
                  <th>状态</th>
                  <th>注册时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="user in users" :key="user.id">
                  <td>{{ user.id }}</td>
                  <td>{{ user.username }}</td>
                  <td><span :class="['status-badge', user.status === 1 ? 'status-on' : 'status-off']">{{ userStatusText(user.status) }}</span></td>
                  <td>{{ user.createdAt }}</td>
                  <td>
                    <button class="text-button" @click="toggleUserStatus(user)">
                      {{ user.status === 1 ? '禁用' : '启用' }}
                    </button>
                  </td>
                </tr>
                <tr v-if="users.length === 0">
                  <td colspan="5" class="table-empty">暂无用户数据</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="pager" v-if="pagination.users.total > 0">
            <span>第 {{ pagination.users.page }} / {{ totalPages('users') }} 页，共 {{ pagination.users.total }} 条</span>
            <div class="pager-actions">
              <button class="btn btn-secondary" :disabled="pagination.users.page <= 1 || loadingState.users" @click="changePage('users', pagination.users.page - 1)">上一页</button>
              <button class="btn btn-secondary" :disabled="pagination.users.page >= totalPages('users') || loadingState.users" @click="changePage('users', pagination.users.page + 1)">下一页</button>
            </div>
          </div>
        </div>
      </section>

      <section v-if="activeTab === 'stats'" class="admin-section">
        <div class="stats-grid">
          <article class="stat-card">
            <span class="stat-label">待支付订单</span>
            <strong class="stat-value">{{ stats.orderStats.unpaidOrders }}</strong>
            <p class="stat-meta">当前待处理订单量</p>
          </article>
          <article class="stat-card">
            <span class="stat-label">成交订单</span>
            <strong class="stat-value">{{ stats.orderStats.paidOrders }}</strong>
            <p class="stat-meta">今日成交 {{ stats.orderStats.todayPaidOrders }}</p>
          </article>
          <article class="stat-card">
            <span class="stat-label">订单总数</span>
            <strong class="stat-value">{{ stats.orderStats.totalOrders }}</strong>
            <p class="stat-meta">含待支付、已支付、已取消</p>
          </article>
          <article class="stat-card">
            <span class="stat-label">累计销售额</span>
            <strong class="stat-value">{{ formatCurrency(stats.orderStats.totalSales) }}</strong>
            <p class="stat-meta">今日销售额 {{ formatCurrency(stats.orderStats.todaySalesAmount) }}</p>
          </article>
          <article class="stat-card">
            <span class="stat-label">商品概况</span>
            <strong class="stat-value">{{ stats.goodsStats.totalGoods }}</strong>
            <p class="stat-meta">上架 {{ stats.goodsStats.onSaleGoods }} / 总库存 {{ stats.goodsStats.totalStock }}</p>
          </article>
          <article class="stat-card">
            <span class="stat-label">用户概况</span>
            <strong class="stat-value">{{ stats.userStats.totalUsers }}</strong>
            <p class="stat-meta">启用 {{ stats.userStats.enabledUsers }} / 禁用 {{ stats.userStats.disabledUsers }}</p>
          </article>
        </div>

        <div class="panel">
          <div class="panel-header">
            <h2>商品销量排行</h2>
            <div class="panel-side">
              <span class="panel-note">统计结果按变更实时写入 Redis；手动改库后可点右侧重建。</span>
              <button class="btn btn-secondary" @click="handleRebuildStats" :disabled="rebuildingStats || loadingState.stats">
                {{ rebuildingStats ? '重建中...' : '重建统计快照' }}
              </button>
            </div>
          </div>
          <div class="table-shell">
            <table class="admin-table">
              <thead>
                <tr>
                  <th>排名</th>
                  <th>商品</th>
                  <th>销量</th>
                  <th>支付订单数</th>
                  <th>销售额</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(item, index) in stats.salesRanking" :key="`${item.goodsId}-${index}`">
                  <td>{{ index + 1 }}</td>
                  <td>{{ item.goodsName }}</td>
                  <td>{{ item.soldQuantity }}</td>
                  <td>{{ item.orderCount }}</td>
                  <td>{{ formatCurrency(item.salesAmount) }}</td>
                </tr>
                <tr v-if="stats.salesRanking.length === 0">
                  <td colspan="5" class="table-empty">暂无已支付订单数据</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <section v-if="activeTab === 'observability'" class="admin-section">
        <article class="panel ops-hero-panel">
          <div class="panel-header">
            <div>
              <h2>Kafka / Redis 运行态</h2>
              <p class="warmup-desc">业务视角直接看分段库存、超时队列、分区 lag 和 consumer 归属；排查时可一键跳到外部工具。</p>
            </div>
            <div class="ops-tool-links">
              <a
                v-if="observability.toolLinks.akhq"
                :href="observability.toolLinks.akhq"
                target="_blank"
                rel="noreferrer"
                class="btn btn-secondary admin-link"
              >
                打开 AKHQ
              </a>
              <a
                v-if="observability.toolLinks.redisInsight"
                :href="observability.toolLinks.redisInsight"
                target="_blank"
                rel="noreferrer"
                class="btn btn-secondary admin-link"
              >
                打开 Redis Insight
              </a>
            </div>
          </div>

          <div class="stats-grid">
            <article class="stat-card accent-red">
              <span class="stat-label">Redis Key 总数</span>
              <strong class="stat-value">{{ formatCount(observability.redis.dbSize) }}</strong>
              <p class="stat-meta">分段 {{ formatCount(observability.redis.keyspace.segmentKeys) }} / 已购 {{ formatCount(observability.redis.keyspace.boughtKeys) }}</p>
            </article>
            <article class="stat-card accent-amber">
              <span class="stat-label">超时队列</span>
              <strong class="stat-value">{{ formatCount(observability.redis.timeoutQueueSize) }}</strong>
              <p class="stat-meta">到期待处理 {{ formatCount(observability.redis.pendingTimeoutCount) }}</p>
            </article>
            <article class="stat-card accent-cyan">
              <span class="stat-label">Kafka 总 Lag</span>
              <strong class="stat-value">{{ formatCount(observability.kafka.totalLag) }}</strong>
              <p class="stat-meta">消费组 {{ groupStateText(observability.kafka.groupState) }}</p>
            </article>
            <article class="stat-card accent-slate">
              <span class="stat-label">DLQ 深度</span>
              <strong class="stat-value">{{ formatCount(observability.kafka.dlqDepth) }}</strong>
              <p class="stat-meta">活跃消费者 {{ formatCount(observability.kafka.activeMemberCount) }}</p>
            </article>
          </div>
        </article>

        <div v-if="observability.redisError || observability.kafkaError" class="ops-error-grid">
          <div v-if="observability.redisError" class="alert alert-error">Redis 观测获取失败：{{ observability.redisError }}</div>
          <div v-if="observability.kafkaError" class="alert alert-error">Kafka 观测获取失败：{{ observability.kafkaError }}</div>
        </div>

        <div class="observability-grid">
          <article class="panel">
            <div class="panel-header">
              <h2>Redis 键空间</h2>
              <span :class="['status-badge', observability.redis.adminStatsReady ? 'status-on' : 'status-pending']">
                {{ observability.redis.adminStatsReady ? '统计快照可读' : '统计快照待重建' }}
              </span>
            </div>
            <div class="keyspace-grid">
              <div class="kv-card">
                <span>总键数</span>
                <strong>{{ formatCount(observability.redis.keyspace.totalKeys) }}</strong>
              </div>
              <div class="kv-card">
                <span>库存分段</span>
                <strong>{{ formatCount(observability.redis.keyspace.segmentKeys) }}</strong>
              </div>
              <div class="kv-card">
                <span>已购记录</span>
                <strong>{{ formatCount(observability.redis.keyspace.boughtKeys) }}</strong>
              </div>
              <div class="kv-card">
                <span>已处理记录</span>
                <strong>{{ formatCount(observability.redis.keyspace.processedKeys) }}</strong>
              </div>
              <div class="kv-card">
                <span>商品状态</span>
                <strong>{{ formatCount(observability.redis.keyspace.goodsStatusKeys) }}</strong>
              </div>
              <div class="kv-card">
                <span>后台快照</span>
                <strong>{{ formatCount(observability.redis.keyspace.adminStatsKeys) }}</strong>
              </div>
            </div>

            <div class="panel-header slim">
              <h3>预热锁</h3>
              <span class="panel-note">当前 {{ formatCount(observability.redis.warmupLocks.length) }} 个</span>
            </div>
            <div v-if="observability.redis.warmupLocks.length === 0" class="table-empty">当前没有预热锁</div>
            <div v-else class="mini-list">
              <div v-for="lock in observability.redis.warmupLocks" :key="lock.key" class="mini-item">
                <strong>{{ lock.target === 'all' ? '全量预热' : `商品 ${lock.target}` }}</strong>
                <span>{{ lock.key }}</span>
                <span>剩余 {{ formatCount(lock.ttlSec) }} 秒</span>
              </div>
            </div>
          </article>

          <article class="panel">
            <div class="panel-header">
              <h2>Kafka 消费组</h2>
              <span :class="['status-badge', observability.kafka.totalLag > 0 ? 'status-pending' : 'status-on']">
                {{ groupStateText(observability.kafka.groupState) }}
              </span>
            </div>
            <div class="keyspace-grid">
              <div class="kv-card wide">
                <span>Topic</span>
                <strong>{{ observability.kafka.topic || '-' }}</strong>
              </div>
              <div class="kv-card wide">
                <span>Consumer Group</span>
                <strong>{{ observability.kafka.group || '-' }}</strong>
              </div>
              <div class="kv-card">
                <span>分区数</span>
                <strong>{{ formatCount(observability.kafka.partitionCount) }}</strong>
              </div>
              <div class="kv-card">
                <span>最新 Offset 汇总</span>
                <strong>{{ formatCount(observability.kafka.totalLatestOffset) }}</strong>
              </div>
              <div class="kv-card">
                <span>已提交 Offset 汇总</span>
                <strong>{{ formatCount(observability.kafka.totalCommittedOffset) }}</strong>
              </div>
              <div class="kv-card wide">
                <span>Brokers</span>
                <strong>{{ observability.kafka.brokers.join(', ') || '-' }}</strong>
              </div>
            </div>

            <div class="panel-header slim">
              <h3>消费者成员</h3>
              <span class="panel-note">按分区归属展示</span>
            </div>
            <div v-if="observability.kafka.members.length === 0" class="member-frame member-frame-empty">
              <div class="table-empty">当前没有活跃消费者成员</div>
            </div>
            <div v-else class="mini-list member-frame member-list-scroll">
              <div v-for="member in observability.kafka.members" :key="member.memberId" class="mini-item">
                <strong>{{ member.clientId || member.memberId }}</strong>
                <span>{{ member.clientHost || '未知来源' }}</span>
                <span>分区 {{ member.partitions.length ? member.partitions.join(', ') : '-' }}</span>
              </div>
            </div>
          </article>
        </div>

        <article class="panel">
          <div class="panel-header">
            <h2>Redis 商品运行态</h2>
            <span class="panel-note">看分段库存、已购与已处理数量是否对齐。</span>
          </div>
          <div class="table-shell">
            <table class="admin-table">
              <thead>
                <tr>
                  <th>商品</th>
                  <th>状态</th>
                  <th>总库存</th>
                  <th>已购</th>
                  <th>已处理</th>
                  <th>待落库</th>
                  <th>分段库存</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in observability.redis.goods" :key="item.goodsId">
                  <td>
                    <div class="table-title">{{ item.goodsName }}</div>
                    <div class="table-subtitle">ID {{ item.goodsId }}</div>
                  </td>
                  <td>
                    <span :class="['status-badge', item.onSale ? 'status-on' : 'status-off']">
                      {{ item.onSale ? '上架' : '下架' }}
                    </span>
                  </td>
                  <td>{{ formatCount(item.totalStock) }}</td>
                  <td>{{ formatCount(item.boughtQuantity) }} / {{ formatCount(item.boughtUsers) }} 人</td>
                  <td>{{ formatCount(item.processedQuantity) }} / {{ formatCount(item.processedUsers) }} 人</td>
                  <td>
                    <span :class="['status-badge', item.pendingQuantity > 0 ? 'status-pending' : 'status-on']">
                      {{ formatCount(item.pendingQuantity) }}
                    </span>
                  </td>
                  <td>
                    <div class="segment-stack">
                      <span
                        v-for="segment in visibleSegmentStocks(item.segmentStocks)"
                        :key="`${item.goodsId}-${segment.segmentId}`"
                        class="segment-pill"
                      >
                        S{{ segment.segmentId }}: {{ formatCount(segment.stock) }}
                      </span>
                      <span v-if="remainingSegmentCount(item.segmentStocks) > 0" class="segment-pill muted">
                        +{{ remainingSegmentCount(item.segmentStocks) }} 段
                      </span>
                    </div>
                  </td>
                </tr>
                <tr v-if="observability.redis.goods.length === 0">
                  <td colspan="7" class="table-empty">暂无 Redis 商品运行态</td>
                </tr>
              </tbody>
            </table>
          </div>
        </article>

        <article class="panel">
          <div class="panel-header">
            <h2>Kafka 分区运行态</h2>
            <span class="panel-note">按 partition 看 leader、offset、lag 和消费者归属。</span>
          </div>
          <div class="toolbar partition-toolbar">
            <input
              v-model.trim="kafkaPartitionFilters.keyword"
              class="form-input toolbar-input"
              placeholder="筛选 Partition / Consumer / Host / Leader"
            />
            <select v-model="kafkaPartitionFilters.lag" class="form-input toolbar-select">
              <option value="all">全部 Lag</option>
              <option value="lagged">有积压</option>
              <option value="clean">无积压</option>
              <option value="high">高积压</option>
            </select>
            <select v-model="kafkaPartitionFilters.assignment" class="form-input toolbar-select">
              <option value="all">全部归属</option>
              <option value="assigned">已分配</option>
              <option value="unassigned">未分配</option>
            </select>
            <button class="btn btn-secondary" @click="resetKafkaPartitionFilters">重置</button>
            <span class="panel-note partition-count">显示 {{ formatCount(filteredKafkaPartitions.length) }} / {{ formatCount(observability.kafka.partitions.length) }}</span>
          </div>
          <div class="table-shell partition-table-frame">
            <table class="admin-table partition-table">
              <thead>
                <tr>
                  <th>Partition</th>
                  <th>Leader</th>
                  <th>Earliest</th>
                  <th>Latest</th>
                  <th>Committed</th>
                  <th>Lag</th>
                  <th>消费者</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="partition in filteredKafkaPartitions" :key="partition.partition">
                  <td>{{ partition.partition }}</td>
                  <td>{{ partition.leader }}</td>
                  <td>{{ formatCount(partition.earliestOffset) }}</td>
                  <td>{{ formatCount(partition.latestOffset) }}</td>
                  <td>{{ partition.committedOffset >= 0 ? formatCount(partition.committedOffset) : '-' }}</td>
                  <td>
                    <span :class="['status-badge', lagStatusClass(partition.lag)]">
                      {{ formatCount(partition.lag) }}
                    </span>
                  </td>
                  <td>
                    <div class="table-title">{{ partition.clientId || partition.memberId || '-' }}</div>
                    <div class="table-subtitle">{{ partition.clientHost || '未分配' }}</div>
                  </td>
                </tr>
                <tr v-if="filteredKafkaPartitions.length === 0">
                  <td colspan="7" class="table-empty">
                    {{ observability.kafka.partitions.length === 0 ? '暂无 Kafka 分区运行态' : '没有匹配的分区' }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </article>
      </section>

      <section v-if="activeTab === 'warmup'" class="admin-section">
        <div class="warmup-grid">
          <article class="panel warmup-card">
            <div class="panel-header">
              <h2>全量预热</h2>
            </div>
            <p class="warmup-desc">将所有商品的 MySQL 库存重新同步到 Redis，适合启动后或批量修改库存后执行。</p>
            <button class="btn btn-primary" @click="handleWarmUpAll" :disabled="warmingAll">
              {{ warmingAll ? '预热中...' : '执行全量预热' }}
            </button>
          </article>

          <article class="panel warmup-card">
            <div class="panel-header">
              <h2>单商品预热</h2>
            </div>
            <p class="warmup-desc">按商品 ID 触发单次库存预热，适合局部补货或单品库存调整后执行。</p>
            <div class="toolbar compact">
              <input v-model="warmupGoodsId" type="number" min="1" step="1" class="form-input toolbar-input" placeholder="请输入商品 ID" />
              <button class="btn btn-primary" @click="handleManualWarmUp" :disabled="warmingSingle">
                {{ warmingSingle ? '预热中...' : '执行单品预热' }}
              </button>
            </div>
          </article>
        </div>
      </section>
    </div>

    <Toast ref="toast" />
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  adminCreateGoods,
  adminDeleteGoods,
  adminGetGoods,
  adminGetObservability,
  adminGetOrderDetail,
  adminGetOrders,
  adminRebuildStats,
  adminGetStats,
  adminGetUsers,
  adminPing,
  adminUpdateGoods,
  adminUpdateUserStatus,
  adminWarmUpAll,
  adminWarmUpGoods
} from '../api'
import Toast from '../components/Toast.vue'

const ADMIN_SECRET_KEY = 'admin_secret'

const toast = ref(null)
const tabs = [
  { key: 'goods', label: '商品管理' },
  { key: 'orders', label: '订单管理' },
  { key: 'users', label: '用户管理' },
  { key: 'stats', label: '数据统计' },
  { key: 'observability', label: '中间件运行监控' },
  { key: 'warmup', label: '库存预热' }
]

const secretInput = ref(localStorage.getItem(ADMIN_SECRET_KEY) || '')
const adminSecret = ref(localStorage.getItem(ADMIN_SECRET_KEY) || '')
const verified = ref(false)
const authLoading = ref(false)
const authError = ref('')
const activeTab = ref('goods')

const loadingState = reactive({
  stats: false,
  observability: false,
  goods: false,
  orders: false,
  users: false
})

const loadedTabs = reactive({
  stats: false,
  observability: false,
  goods: false,
  orders: false,
  users: false
})

const pagination = reactive({
  goods: { page: 1, pageSize: 20, total: 0 },
  orders: { page: 1, pageSize: 20, total: 0 },
  users: { page: 1, pageSize: 20, total: 0 }
})

const goodsList = ref([])
const orders = ref([])
const users = ref([])
const selectedOrder = ref(null)
const savingGoods = ref(false)
const rebuildingStats = ref(false)
const warmingAll = ref(false)
const warmingSingle = ref(false)
const warmupGoodsId = ref('')

const stats = reactive({
  orderStats: {
    totalOrders: 0,
    paidOrders: 0,
    unpaidOrders: 0,
    cancelledOrders: 0,
    totalSales: 0,
    todayPaidOrders: 0,
    todaySalesAmount: 0
  },
  userStats: {
    totalUsers: 0,
    enabledUsers: 0,
    disabledUsers: 0
  },
  goodsStats: {
    totalGoods: 0,
    onSaleGoods: 0,
    totalStock: 0
  },
  salesRanking: []
})

const observability = reactive({
  redisError: '',
  kafkaError: '',
  toolLinks: {
    akhq: '',
    redisInsight: ''
  },
  redis: {
    dbSize: 0,
    adminStatsReady: false,
    timeoutQueueSize: 0,
    pendingTimeoutCount: 0,
    keyspace: {
      totalKeys: 0,
      segmentKeys: 0,
      boughtKeys: 0,
      processedKeys: 0,
      goodsStatusKeys: 0,
      userStatusKeys: 0,
      adminStatsKeys: 0
    },
    warmupLocks: [],
    goods: []
  },
  kafka: {
    brokers: [],
    topic: '',
    group: '',
    groupState: 'unknown',
    partitionCount: 0,
    activeMemberCount: 0,
    totalLatestOffset: 0,
    totalCommittedOffset: 0,
    totalLag: 0,
    dlqTopic: '',
    dlqDepth: 0,
    members: [],
    partitions: []
  }
})

const resolveToolUrl = (fallback, port) => {
  try {
    const current = new URL(window.location.href)
    if (!fallback) {
      return `${current.protocol}//${current.hostname}:${port}`
    }

    const target = new URL(fallback)
    const isLocalFallback = ['localhost', '127.0.0.1'].includes(target.hostname)
    const isRemoteCurrent = !['localhost', '127.0.0.1'].includes(current.hostname)
    if (isLocalFallback && isRemoteCurrent) {
      target.protocol = current.protocol
      target.hostname = current.hostname
    }
    return target.toString().replace(/\/$/, '')
  } catch {
    return fallback || `http://localhost:${port}`
  }
}

const goodsFilters = reactive({
  keyword: '',
  status: ''
})

const orderFilters = reactive({
  keyword: '',
  status: ''
})

const userFilters = reactive({
  keyword: '',
  status: ''
})

const kafkaPartitionFilters = reactive({
  keyword: '',
  lag: 'all',
  assignment: 'all'
})

const goodsForm = reactive({
  id: null,
  productName: '',
  description: '',
  price: 0,
  stock: 0,
  status: 1
})

const tabLoading = computed(() => loadingState[activeTab.value] || false)

const showToast = (message, type = 'info') => {
  toast.value?.show(message, type)
}

const normalizeGoods = items => (items || []).map(item => ({
  id: item.ID,
  productName: item.ProductName,
  description: item.Description,
  stock: Number(item.Stock || 0),
  price: Number(item.Price || 0),
  status: Number(item.Status || 0),
  createdAt: item.CreatedAt,
  updatedAt: item.UpdatedAt
}))

const normalizeOrders = items => (items || []).map(item => ({
  id: item.ID,
  userId: item.UserID,
  username: item.username,
  goodsId: item.GoodsID,
  goodsName: item.goods_name,
  quantity: Number(item.Quantity || 0),
  payAmount: Number(item.PayAmount || 0),
  status: Number(item.Status || 0),
  requestTime: item.RequestTime,
  createTime: item.CreateTime,
  payTime: item.PayTime,
  cancelTime: item.CancelTime
}))

const normalizeUsers = items => (items || []).map(item => ({
  id: item.id,
  username: item.username,
  status: Number(item.status || 0),
  createdAt: item.created_at
}))

const applyStats = payload => {
  const next = payload || {}
  Object.assign(stats.orderStats, {
    totalOrders: Number(next.order_stats?.total_orders || 0),
    paidOrders: Number(next.order_stats?.paid_orders || 0),
    unpaidOrders: Number(next.order_stats?.unpaid_orders || 0),
    cancelledOrders: Number(next.order_stats?.cancelled_orders || 0),
    totalSales: Number(next.order_stats?.total_sales || 0),
    todayPaidOrders: Number(next.order_stats?.today_paid_orders || 0),
    todaySalesAmount: Number(next.order_stats?.today_sales_amount || 0)
  })
  Object.assign(stats.userStats, {
    totalUsers: Number(next.user_stats?.total_users || 0),
    enabledUsers: Number(next.user_stats?.enabled_users || 0),
    disabledUsers: Number(next.user_stats?.disabled_users || 0)
  })
  Object.assign(stats.goodsStats, {
    totalGoods: Number(next.goods_stats?.total_goods || 0),
    onSaleGoods: Number(next.goods_stats?.on_sale_goods || 0),
    totalStock: Number(next.goods_stats?.total_stock || 0)
  })
  stats.salesRanking = (next.sales_ranking || []).map(item => ({
    goodsId: item.goods_id,
    goodsName: item.goods_name,
    soldQuantity: Number(item.sold_quantity || 0),
    orderCount: Number(item.order_count || 0),
    salesAmount: Number(item.sales_amount || 0)
  }))
}

const applyObservability = payload => {
  const next = payload || {}
  observability.redisError = next.redis_error || ''
  observability.kafkaError = next.kafka_error || ''
  observability.toolLinks.akhq = resolveToolUrl(next.tool_links?.akhq || '', 8086)
  observability.toolLinks.redisInsight = resolveToolUrl(next.tool_links?.redis_insight || '', 5540)

  Object.assign(observability.redis.keyspace, {
    totalKeys: Number(next.redis?.keyspace?.total_keys || 0),
    segmentKeys: Number(next.redis?.keyspace?.segment_keys || 0),
    boughtKeys: Number(next.redis?.keyspace?.bought_keys || 0),
    processedKeys: Number(next.redis?.keyspace?.processed_keys || 0),
    goodsStatusKeys: Number(next.redis?.keyspace?.goods_status_keys || 0),
    userStatusKeys: Number(next.redis?.keyspace?.user_status_keys || 0),
    adminStatsKeys: Number(next.redis?.keyspace?.admin_stats_keys || 0)
  })

  Object.assign(observability.redis, {
    dbSize: Number(next.redis?.db_size || 0),
    adminStatsReady: Boolean(next.redis?.admin_stats_ready),
    timeoutQueueSize: Number(next.redis?.timeout_queue_size || 0),
    pendingTimeoutCount: Number(next.redis?.pending_timeout_count || 0),
    warmupLocks: (next.redis?.warmup_locks || []).map(item => ({
      key: item.key,
      target: item.target,
      ttlSec: Number(item.ttl_sec || 0)
    })),
    goods: (next.redis?.goods || []).map(item => ({
      goodsId: Number(item.goods_id || 0),
      goodsName: item.goods_name,
      onSale: Boolean(item.on_sale),
      totalStock: Number(item.total_stock || 0),
      boughtUsers: Number(item.bought_users || 0),
      boughtQuantity: Number(item.bought_quantity || 0),
      processedUsers: Number(item.processed_users || 0),
      processedQuantity: Number(item.processed_quantity || 0),
      pendingQuantity: Number(item.pending_quantity || 0),
      segmentStocks: (item.segment_stocks || []).map(segment => ({
        segmentId: Number(segment.segment_id || 0),
        stock: Number(segment.stock || 0)
      }))
    }))
  })

  Object.assign(observability.kafka, {
    brokers: next.kafka?.brokers || [],
    topic: next.kafka?.topic || '',
    group: next.kafka?.group || '',
    groupState: next.kafka?.group_state || 'unknown',
    partitionCount: Number(next.kafka?.partition_count || 0),
    activeMemberCount: Number(next.kafka?.active_member_count || 0),
    totalLatestOffset: Number(next.kafka?.total_latest_offset || 0),
    totalCommittedOffset: Number(next.kafka?.total_committed_offset || 0),
    totalLag: Number(next.kafka?.total_lag || 0),
    dlqTopic: next.kafka?.dlq_topic || '',
    dlqDepth: Number(next.kafka?.dlq_depth || 0),
    members: (next.kafka?.members || []).map(member => ({
      memberId: member.member_id,
      clientId: member.client_id,
      clientHost: member.client_host,
      partitions: member.partitions || []
    })),
    partitions: (next.kafka?.partitions || []).map(partition => ({
      partition: Number(partition.partition || 0),
      leader: partition.leader || '-',
      earliestOffset: Number(partition.earliest_offset || 0),
      latestOffset: Number(partition.latest_offset || 0),
      committedOffset: Number(partition.committed_offset ?? -1),
      lag: Number(partition.lag || 0),
      memberId: partition.member_id || '',
      clientId: partition.client_id || '',
      clientHost: partition.client_host || ''
    }))
  })
}

const formatCurrency = value => `¥${Number(value || 0).toFixed(2)}`

const formatCount = value => Number(value || 0).toLocaleString('zh-CN')

const formatTime = value => {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hour = String(date.getHours()).padStart(2, '0')
  const minute = String(date.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day} ${hour}:${minute}`
}

const goodsStatusText = status => (status === 1 ? '上架' : '下架')
const orderStatusText = status => ({ 0: '待支付', 1: '已支付', 2: '已取消' }[status] || '未知')
const orderStatusClass = status => ({ 0: 'status-pending', 1: 'status-on', 2: 'status-off' }[status] || 'status-off')
const userStatusText = status => (status === 1 ? '启用' : '禁用')
const groupStateText = state => ({
  Stable: '稳定',
  PreparingRebalance: '重平衡中',
  CompletingRebalance: '完成重平衡',
  Empty: '空闲',
  Dead: '已停用',
  unknown: '未知'
}[state] || state || '未知')

const lagStatusClass = lag => {
  if (lag > 1000) return 'status-off'
  if (lag > 0) return 'status-pending'
  return 'status-on'
}

const visibleSegmentStocks = segmentStocks => {
  const nonZero = (segmentStocks || []).filter(item => item.stock > 0)
  const source = nonZero.length > 0 ? nonZero : (segmentStocks || [])
  return source.slice(0, 8)
}

const remainingSegmentCount = segmentStocks => {
  const nonZero = (segmentStocks || []).filter(item => item.stock > 0)
  const source = nonZero.length > 0 ? nonZero : (segmentStocks || [])
  return Math.max(source.length - 8, 0)
}

const filteredKafkaPartitions = computed(() => {
  const keyword = kafkaPartitionFilters.keyword.trim().toLowerCase()

  return (observability.kafka.partitions || []).filter(partition => {
    if (kafkaPartitionFilters.lag === 'lagged' && partition.lag <= 0) return false
    if (kafkaPartitionFilters.lag === 'clean' && partition.lag !== 0) return false
    if (kafkaPartitionFilters.lag === 'high' && partition.lag <= 1000) return false

    const assigned = Boolean(partition.memberId || partition.clientId || partition.clientHost)
    if (kafkaPartitionFilters.assignment === 'assigned' && !assigned) return false
    if (kafkaPartitionFilters.assignment === 'unassigned' && assigned) return false

    if (!keyword) return true

    const haystack = [
      String(partition.partition),
      partition.leader,
      partition.memberId,
      partition.clientId,
      partition.clientHost
    ]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()

    return haystack.includes(keyword)
  })
})

const totalPages = tab => {
  const current = pagination[tab]
  return Math.max(1, Math.ceil(current.total / current.pageSize))
}

const buildListParams = (filters, pager) => {
  const params = {
    page: pager.page,
    page_size: pager.pageSize
  }
  if (filters.keyword.trim()) params.keyword = filters.keyword.trim()
  if (filters.status !== '') params.status = filters.status
  return params
}

const markStatsDirty = () => {
  loadedTabs.stats = false
}

const handleAdminError = (error, fallbackMessage) => {
  if (error.response?.status === 403) {
    authError.value = '管理密钥无效，请重新输入'
    logoutAdmin(false)
    return
  }
  showToast(error.response?.data?.msg || fallbackMessage, 'error')
}

const fetchStats = async () => {
  loadingState.stats = true
  try {
    const res = await adminGetStats(adminSecret.value)
    if (res.data.code === 0) {
      applyStats(res.data.data)
      loadedTabs.stats = true
    } else {
      showToast(res.data.msg || '获取统计数据失败', 'error')
    }
  } catch (error) {
    handleAdminError(error, '获取统计数据失败')
  } finally {
    loadingState.stats = false
  }
}

const fetchObservability = async () => {
  loadingState.observability = true
  try {
    const res = await adminGetObservability(adminSecret.value)
    if (res.data.code === 0) {
      applyObservability(res.data.data)
      loadedTabs.observability = true
    } else {
      showToast(res.data.msg || '获取中间件运行监控数据失败', 'error')
    }
  } catch (error) {
    handleAdminError(error, '获取中间件运行监控数据失败')
  } finally {
    loadingState.observability = false
  }
}

const handleRebuildStats = async () => {
  rebuildingStats.value = true
  try {
    const res = await adminRebuildStats(adminSecret.value)
    if (res.data.code === 0) {
      applyStats(res.data.data)
      loadedTabs.stats = true
      showToast('统计快照已重建', 'success')
    } else {
      showToast(res.data.msg || '重建统计快照失败', 'error')
    }
  } catch (error) {
    handleAdminError(error, '重建统计快照失败')
  } finally {
    rebuildingStats.value = false
  }
}

const fetchGoods = async (page = pagination.goods.page) => {
  loadingState.goods = true
  pagination.goods.page = page
  try {
    const res = await adminGetGoods(adminSecret.value, buildListParams(goodsFilters, pagination.goods))
    if (res.data.code === 0) {
      const payload = res.data.data || {}
      goodsList.value = normalizeGoods(payload.items)
      pagination.goods.total = Number(payload.total || 0)
      pagination.goods.page = Number(payload.page || page)
      loadedTabs.goods = true
    } else {
      showToast(res.data.msg || '获取商品列表失败', 'error')
    }
  } catch (error) {
    handleAdminError(error, '获取商品列表失败')
  } finally {
    loadingState.goods = false
  }
}

const fetchOrders = async (page = pagination.orders.page) => {
  loadingState.orders = true
  pagination.orders.page = page
  try {
    const res = await adminGetOrders(adminSecret.value, buildListParams(orderFilters, pagination.orders))
    if (res.data.code === 0) {
      const payload = res.data.data || {}
      orders.value = normalizeOrders(payload.items)
      pagination.orders.total = Number(payload.total || 0)
      pagination.orders.page = Number(payload.page || page)
      loadedTabs.orders = true
    } else {
      showToast(res.data.msg || '获取订单列表失败', 'error')
    }
  } catch (error) {
    handleAdminError(error, '获取订单列表失败')
  } finally {
    loadingState.orders = false
  }
}

const fetchUsers = async (page = pagination.users.page) => {
  loadingState.users = true
  pagination.users.page = page
  try {
    const res = await adminGetUsers(adminSecret.value, buildListParams(userFilters, pagination.users))
    if (res.data.code === 0) {
      const payload = res.data.data || {}
      users.value = normalizeUsers(payload.items)
      pagination.users.total = Number(payload.total || 0)
      pagination.users.page = Number(payload.page || page)
      loadedTabs.users = true
    } else {
      showToast(res.data.msg || '获取用户列表失败', 'error')
    }
  } catch (error) {
    handleAdminError(error, '获取用户列表失败')
  } finally {
    loadingState.users = false
  }
}

const fetchOrderDetail = async orderId => {
  try {
    const res = await adminGetOrderDetail(adminSecret.value, orderId)
    if (res.data.code === 0) {
      selectedOrder.value = normalizeOrders([res.data.data])[0] || null
    } else {
      showToast(res.data.msg || '获取订单详情失败', 'error')
    }
  } catch (error) {
    handleAdminError(error, '获取订单详情失败')
  }
}

const ensureTabLoaded = async (tab, force = false) => {
  if (!verified.value) return
  if (!force && loadedTabs[tab]) return

  switch (tab) {
    case 'goods':
      await fetchGoods(force ? 1 : pagination.goods.page)
      break
    case 'orders':
      await fetchOrders(force ? 1 : pagination.orders.page)
      break
    case 'users':
      await fetchUsers(force ? 1 : pagination.users.page)
      break
    case 'stats':
      await fetchStats()
      break
    case 'observability':
      await fetchObservability()
      break
    default:
      break
  }
}

const changePage = async (tab, nextPage) => {
  if (nextPage < 1 || nextPage > totalPages(tab)) return

  switch (tab) {
    case 'goods':
      await fetchGoods(nextPage)
      break
    case 'orders':
      await fetchOrders(nextPage)
      break
    case 'users':
      await fetchUsers(nextPage)
      break
    default:
      break
  }
}

const resetGoodsForm = () => {
  goodsForm.id = null
  goodsForm.productName = ''
  goodsForm.description = ''
  goodsForm.price = 0
  goodsForm.stock = 0
  goodsForm.status = 1
}

const prepareCreateGoods = () => {
  resetGoodsForm()
}

const editGoods = goods => {
  goodsForm.id = goods.id
  goodsForm.productName = goods.productName
  goodsForm.description = goods.description
  goodsForm.price = goods.price
  goodsForm.stock = goods.stock
  goodsForm.status = goods.status
}

const submitGoodsForm = async () => {
  if (!goodsForm.productName.trim()) {
    showToast('请输入商品名称', 'error')
    return
  }
  if (goodsForm.stock < 0 || goodsForm.price < 0) {
    showToast('价格和库存不能小于 0', 'error')
    return
  }

  const payload = {
    product_name: goodsForm.productName.trim(),
    description: goodsForm.description.trim(),
    stock: Number(goodsForm.stock),
    price: Number(goodsForm.price),
    status: Number(goodsForm.status)
  }

  savingGoods.value = true
  try {
    const res = goodsForm.id
      ? await adminUpdateGoods(adminSecret.value, goodsForm.id, payload)
      : await adminCreateGoods(adminSecret.value, payload)

    if (res.data.code === 0) {
      showToast(goodsForm.id ? '商品已更新' : '商品已创建', 'success')
      resetGoodsForm()
      markStatsDirty()
      await fetchGoods(1)
    } else {
      showToast(res.data.msg || '保存商品失败', 'error')
    }
  } catch (error) {
    handleAdminError(error, '保存商品失败')
  } finally {
    savingGoods.value = false
  }
}

const removeGoods = async goods => {
  if (!window.confirm(`确认删除商品“${goods.productName}”吗？`)) return
  try {
    const res = await adminDeleteGoods(adminSecret.value, goods.id)
    if (res.data.code === 0) {
      showToast('商品已删除', 'success')
      markStatsDirty()
      await fetchGoods(1)
    } else {
      showToast(res.data.msg || '删除商品失败', 'error')
    }
  } catch (error) {
    handleAdminError(error, '删除商品失败')
  }
}

const handleWarmUpSingle = async goodsId => {
  warmingSingle.value = true
  try {
    const res = await adminWarmUpGoods(adminSecret.value, goodsId)
    if (res.data.code === 0) {
      showToast(`商品 ${goodsId} 预热完成`, 'success')
    } else {
      showToast(res.data.msg || '单商品预热失败', 'error')
    }
  } catch (error) {
    handleAdminError(error, '单商品预热失败')
  } finally {
    warmingSingle.value = false
  }
}

const handleManualWarmUp = async () => {
  const goodsId = Number(warmupGoodsId.value)
  if (!goodsId) {
    showToast('请输入有效的商品 ID', 'error')
    return
  }
  await handleWarmUpSingle(goodsId)
}

const handleWarmUpAll = async () => {
  warmingAll.value = true
  try {
    const res = await adminWarmUpAll(adminSecret.value)
    if (res.data.code === 0) {
      showToast(`全量预热完成，共处理 ${res.data.data.count} 个商品`, 'success')
      if (loadedTabs.goods) {
        await fetchGoods(pagination.goods.page)
      }
    } else {
      showToast(res.data.msg || '全量预热失败', 'error')
    }
  } catch (error) {
    handleAdminError(error, '全量预热失败')
  } finally {
    warmingAll.value = false
  }
}

const toggleUserStatus = async user => {
  const nextStatus = user.status === 1 ? 0 : 1
  try {
    const res = await adminUpdateUserStatus(adminSecret.value, user.id, nextStatus)
    if (res.data.code === 0) {
      showToast(`用户已${nextStatus === 1 ? '启用' : '禁用'}`, 'success')
      markStatsDirty()
      await fetchUsers(pagination.users.page)
    } else {
      showToast(res.data.msg || '更新用户状态失败', 'error')
    }
  } catch (error) {
    handleAdminError(error, '更新用户状态失败')
  }
}

const refreshCurrentTab = async () => {
  await ensureTabLoaded(activeTab.value, true)
}

const handleGoodsSearch = async () => {
  await fetchGoods(1)
}

const handleOrderSearch = async () => {
  selectedOrder.value = null
  await fetchOrders(1)
}

const handleUserSearch = async () => {
  await fetchUsers(1)
}

const resetKafkaPartitionFilters = () => {
  kafkaPartitionFilters.keyword = ''
  kafkaPartitionFilters.lag = 'all'
  kafkaPartitionFilters.assignment = 'all'
}

const resetGoodsFilters = async () => {
  goodsFilters.keyword = ''
  goodsFilters.status = ''
  await fetchGoods(1)
}

const resetOrderFilters = async () => {
  orderFilters.keyword = ''
  orderFilters.status = ''
  selectedOrder.value = null
  await fetchOrders(1)
}

const resetUserFilters = async () => {
  userFilters.keyword = ''
  userFilters.status = ''
  await fetchUsers(1)
}

const handleAdminLogin = async () => {
  const secret = secretInput.value.trim()
  if (!secret) {
    authError.value = '请输入管理密钥'
    return
  }

  authLoading.value = true
  authError.value = ''
  try {
    const res = await adminPing(secret)
    if (res.data.code === 0) {
      adminSecret.value = secret
      verified.value = true
      localStorage.setItem(ADMIN_SECRET_KEY, secret)
      await ensureTabLoaded(activeTab.value, true)
      showToast('已进入后台管理', 'success')
    } else {
      authError.value = res.data.msg || '验证失败'
    }
  } catch (error) {
    authError.value = error.response?.data?.msg || '验证失败'
  } finally {
    authLoading.value = false
  }
}

const logoutAdmin = (notify = true) => {
  verified.value = false
  adminSecret.value = ''
  localStorage.removeItem(ADMIN_SECRET_KEY)
  secretInput.value = ''
  authError.value = ''
  loadedTabs.stats = false
  loadedTabs.observability = false
  loadedTabs.goods = false
  loadedTabs.orders = false
  loadedTabs.users = false
  if (notify) {
    showToast('已退出后台管理', 'info')
  }
}

watch(activeTab, async tab => {
  if (verified.value) {
    await ensureTabLoaded(tab)
  }
})

onMounted(async () => {
  if (!adminSecret.value) return

  try {
    const res = await adminPing(adminSecret.value)
    if (res.data.code === 0) {
      verified.value = true
      secretInput.value = adminSecret.value
      await ensureTabLoaded(activeTab.value, true)
    } else {
      logoutAdmin(false)
    }
  } catch (error) {
    logoutAdmin(false)
  }
})
</script>

<style scoped>
.admin-page {
  min-height: 100vh;
  background:
    radial-gradient(circle at top left, rgba(179, 139, 58, 0.14), transparent 30%),
    linear-gradient(180deg, #fbfaf7 0%, #f4f5f5 42%, #edf0f2 100%);
  padding: 24px;
}

.admin-auth-shell,
.admin-shell {
  max-width: 1280px;
  margin: 0 auto;
}

.admin-auth-shell {
  display: flex;
  min-height: calc(100vh - 48px);
  align-items: center;
  justify-content: center;
}

.admin-auth-card {
  width: min(100%, 460px);
  padding: 36px;
  background: rgba(255, 255, 255, 0.88);
  border: 1px solid var(--border);
  border-radius: 24px;
  box-shadow: var(--shadow-md);
  backdrop-filter: blur(20px);
}

.admin-badge,
.admin-eyebrow {
  display: inline-flex;
  align-items: center;
  min-height: 32px;
  padding: 0 12px;
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--accent-strong);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.auth-copy {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 24px;
}

.auth-title,
.admin-title {
  color: var(--text-primary);
  line-height: 1.2;
}

.auth-title {
  font-size: 34px;
}

.auth-desc,
.admin-subtitle {
  color: var(--text-secondary);
  line-height: 1.7;
}

.auth-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.admin-header {
  display: flex;
  justify-content: space-between;
  gap: 24px;
  align-items: flex-start;
  margin-bottom: 24px;
}

.admin-title {
  margin-top: 12px;
  font-size: 32px;
}

.admin-subtitle {
  margin-top: 8px;
  font-size: 17px;
}

.admin-header-actions {
  display: flex;
  gap: 12px;
}

.admin-link {
  text-decoration: none;
}

.admin-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 20px;
}

.tab-button {
  min-height: 44px;
  padding: 0 16px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.74);
  color: var(--text-secondary);
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
}

.tab-button:hover {
  background: rgba(255, 255, 255, 0.96);
  color: var(--text-primary);
}

.tab-button.active {
  border-color: transparent;
  background: var(--text-primary);
  color: #fff;
  box-shadow: 0 16px 30px rgba(23, 23, 23, 0.14);
}

.admin-section {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.panel,
.stat-card {
  background: rgba(255, 255, 255, 0.84);
  border: 1px solid var(--border);
  border-radius: 20px;
  box-shadow: var(--shadow-sm);
  backdrop-filter: blur(18px);
}

.panel {
  padding: 24px;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: center;
  margin-bottom: 18px;
}

.panel-header.slim {
  margin-bottom: 16px;
}

.panel-header h2,
.panel-header h3 {
  color: var(--text-primary);
}

.panel-note {
  color: var(--text-muted);
  font-size: 14px;
}

.panel-side {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  flex-wrap: wrap;
}

.ops-tool-links {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.ops-error-grid {
  display: grid;
  gap: 12px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
}

.observability-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20px;
}

.stat-card {
  padding: 24px;
}

.stat-card.accent-red {
  background: linear-gradient(180deg, rgba(255, 244, 240, 0.96) 0%, rgba(255, 255, 255, 0.92) 100%);
}

.stat-card.accent-amber {
  background: linear-gradient(180deg, rgba(255, 249, 237, 0.96) 0%, rgba(255, 255, 255, 0.92) 100%);
}

.stat-card.accent-cyan {
  background: linear-gradient(180deg, rgba(238, 250, 251, 0.96) 0%, rgba(255, 255, 255, 0.92) 100%);
}

.stat-card.accent-slate {
  background: linear-gradient(180deg, rgba(240, 244, 247, 0.96) 0%, rgba(255, 255, 255, 0.92) 100%);
}

.stat-label {
  display: block;
  margin-bottom: 12px;
  color: var(--text-muted);
  font-size: 14px;
  font-weight: 600;
}

.stat-value {
  display: block;
  color: var(--text-primary);
  font-size: 32px;
  line-height: 1.1;
}

.stat-meta {
  margin-top: 10px;
  color: var(--text-secondary);
  font-size: 15px;
}

.keyspace-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.kv-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 16px;
  background: rgba(250, 250, 248, 0.92);
  border: 1px solid rgba(23, 23, 23, 0.06);
  border-radius: 14px;
}

.kv-card.wide {
  grid-column: span 2;
}

.kv-card span {
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 600;
}

.kv-card strong {
  color: var(--text-primary);
  font-size: 16px;
  line-height: 1.6;
  word-break: break-word;
}

.mini-list {
  display: grid;
  gap: 12px;
}

.member-frame {
  min-height: 360px;
  max-height: 360px;
}

.member-frame-empty {
  display: flex;
  align-items: center;
  justify-content: center;
}

.member-list-scroll {
  overflow-y: auto;
  padding-right: 6px;
}

.member-list-scroll::-webkit-scrollbar {
  width: 8px;
}

.member-list-scroll::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: rgba(23, 23, 23, 0.14);
}

.mini-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 16px;
  background: rgba(248, 248, 246, 0.92);
  border: 1px solid rgba(23, 23, 23, 0.06);
  border-radius: 14px;
}

.mini-item strong {
  color: var(--text-primary);
  font-size: 15px;
}

.mini-item span {
  color: var(--text-secondary);
  font-size: 14px;
  line-height: 1.6;
  word-break: break-word;
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 18px;
}

.toolbar.compact {
  margin-bottom: 0;
}

.partition-toolbar {
  align-items: center;
}

.partition-count {
  margin-left: auto;
  white-space: nowrap;
}

.toolbar-input {
  min-width: 220px;
  flex: 1;
}

.toolbar-select {
  width: 160px;
}

.goods-form-card {
  margin-bottom: 18px;
  padding: 20px;
  background: linear-gradient(180deg, rgba(243, 234, 215, 0.52) 0%, rgba(255, 255, 255, 0.92) 100%);
  border: 1px solid rgba(179, 139, 58, 0.18);
  border-radius: 18px;
}

.goods-form-grid,
.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  margin-bottom: 18px;
}

.form-group-full {
  grid-column: 1 / -1;
}

.form-textarea {
  min-height: 120px;
  resize: vertical;
}

.table-shell {
  overflow-x: auto;
}

.partition-table-frame {
  min-height: 520px;
  max-height: 520px;
  overflow-y: auto;
  border: 1px solid rgba(23, 23, 23, 0.06);
  border-radius: 18px;
}

.partition-table-frame::-webkit-scrollbar {
  width: 10px;
  height: 10px;
}

.partition-table-frame::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: rgba(23, 23, 23, 0.14);
}

.admin-table {
  width: 100%;
  border-collapse: collapse;
}

.partition-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: rgba(252, 252, 250, 0.98);
  backdrop-filter: blur(8px);
}

.admin-table th,
.admin-table td {
  padding: 14px 12px;
  border-bottom: 1px solid rgba(23, 23, 23, 0.07);
  text-align: left;
  vertical-align: top;
  font-size: 15px;
}

.admin-table th {
  color: var(--text-muted);
  font-size: 14px;
  font-weight: 700;
  white-space: nowrap;
}

.table-title {
  color: var(--text-primary);
  font-weight: 600;
}

.table-subtitle {
  margin-top: 4px;
  color: var(--text-secondary);
  font-size: 14px;
  line-height: 1.6;
}

.segment-stack {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.segment-pill {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 0 10px;
  border-radius: 999px;
  background: rgba(23, 23, 23, 0.06);
  color: var(--text-primary);
  font-size: 13px;
  font-weight: 600;
}

.segment-pill.muted {
  background: rgba(23, 23, 23, 0.04);
  color: var(--text-secondary);
}

.table-empty {
  padding: 30px 16px;
  color: var(--text-secondary);
  text-align: center;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 0 10px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 700;
}

.status-on {
  background: var(--success-soft);
  color: var(--success);
}

.status-off {
  background: var(--danger-soft);
  color: var(--danger);
}

.status-pending {
  background: var(--warning-soft);
  color: var(--warning);
}

.action-row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.text-button {
  padding: 0;
  border: none;
  background: none;
  color: var(--accent-strong);
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
}

.text-button:hover {
  color: var(--text-primary);
}

.text-button.danger {
  color: var(--danger);
}

.detail-card {
  margin-top: 18px;
  padding: 20px;
  background: rgba(247, 243, 234, 0.72);
  border: 1px solid var(--border);
  border-radius: 18px;
}

.detail-grid div {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 16px;
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid rgba(23, 23, 23, 0.06);
  border-radius: 14px;
}

.detail-grid span {
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 600;
}

.detail-grid strong {
  color: var(--text-primary);
  font-size: 15px;
  line-height: 1.5;
  word-break: break-all;
}

.pager {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-top: 18px;
  color: var(--text-secondary);
  font-size: 14px;
}

.pager-actions {
  display: flex;
  gap: 8px;
}

.warmup-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 20px;
}

.warmup-card {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.warmup-desc {
  color: var(--text-secondary);
  font-size: 16px;
  line-height: 1.7;
}

@media (max-width: 900px) {
  .admin-header {
    flex-direction: column;
  }

  .admin-header-actions {
    width: 100%;
    flex-wrap: wrap;
  }

  .admin-header-actions .btn,
  .admin-header-actions .admin-link {
    flex: 1;
  }

  .goods-form-grid,
  .detail-grid {
    grid-template-columns: 1fr;
  }

  .observability-grid,
  .keyspace-grid {
    grid-template-columns: 1fr;
  }

  .kv-card.wide {
    grid-column: span 1;
  }
}

@media (max-width: 640px) {
  .admin-page {
    padding: 16px;
  }

  .admin-auth-card,
  .panel,
  .stat-card {
    padding: 20px;
  }

  .auth-title,
  .admin-title {
    font-size: 28px;
  }

  .toolbar,
  .pager {
    flex-direction: column;
    align-items: stretch;
  }

  .partition-count {
    margin-left: 0;
  }

  .toolbar-select,
  .toolbar-input {
    width: 100%;
    min-width: 0;
  }

  .pager-actions {
    width: 100%;
  }

  .pager-actions .btn {
    flex: 1;
  }
}
</style>
