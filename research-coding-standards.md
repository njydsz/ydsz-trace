# 互联网大厂及主流开源项目研发规范调研报告

> 调研范围：阿里巴巴、字节跳动、腾讯、Google、Microsoft、Amazon、美团，以及 Spring Framework、Kubernetes 等主流开源项目
> 更新日期：2026 年 1 月

---

## 一、命名规范

### 1.1 通用规则（各厂共识）

| 类型 | 规则 | 示例 |
|------|------|------|
| 类名 / 接口名 | UpperCamelCase（大驼峰），接口可选 `I` 前缀（Microsoft / Spring 不推荐） | `UserService`, `OrderController` |
| 方法名 / 变量名 | lowerCamelCase（小驼峰），动宾结构 | `findById()`, `userName` |
| 常量名 | UPPER_SNAKE_CASE（全大写下划线分隔） | `MAX_RETRY_COUNT`, `DEFAULT_TIMEOUT` |
| 包名 | 全小写，多级域名反写，无下划线 | `com.alibaba.project.module` |
| 抽象类 | 前缀 `Abstract` 或后缀 `Base` | `AbstractListener`, `BaseService` |
| 异常类 | 后缀 `Exception` | `BusinessException` |
| 测试类 | 被测类名 + `Test` / `Tests` / `Spec` | `UserServiceTest` |

### 1.2 各厂细则

**阿里巴巴《Java开发手册》**
- 强制禁止拼音与英文混用（`taobao` 这类品牌名除外），拼音缩写一律禁止
- 抽象类命名使用 `Abstract` 或 `Base` 开头
- 异常类命名使用 `Exception` 结尾
- 枚举类名带 `Enum` 后缀，枚举成员全大写下划线分隔
- 各层命名规约：Service/DAO 层方法前缀——`get`（单个）、`list`（多个）、`page`（分页）、`count`（统计）、`save`/`insert`、`remove`/`delete`、`update`
- DO/DTO/VO/BO 严格按领域对象分层命名后缀

**Google Java Style Guide**
- 类名使用 UpperCamelCase（"class" 泛指 class/enum/interface/record/annotation）
- 测试类以 `Test` 结尾
- 方法名和变量名使用 lowerCamelCase
- 常量使用 UPPER_SNAKE_CASE，且仅用于 `static final` 编译期常量
- 局部变量、参数、字段使用 lowerCamelCase；泛型类型参数使用单个大写字母（`T`, `E`, `K`, `V`）
- 注解命名使用 UpperCamelCase

**Microsoft .NET / C# 编码规范**
- 接口名强制大写 `I` 前缀（`IEnumerable`, `IDisposable`）
- 属性类型以 `Attribute` 结尾
- 枚举类型：非标志用单数名词，标志用复数名词（带 `[Flags]` 特性时）
- 标识符禁止双下划线 `__`（编译器保留）
- 私有实例字段 `_camelCase` 前缀下划线；静态字段 `s_` 前缀（editorconfig 可配置）
- 命名空间遵循反向域名表示法（`Microsoft.AspNetCore.Mvc`）
- 除了简单循环计数器（`i`, `j`, `k`），禁止使用单字母名称

**腾讯 AlloyTeam（前端）**
- 项目/目录/文件名：全小写 + 短横线分隔（`my-project`, `account-model.js`）
- 有复数结构时用复数命名（`scripts/`, `styles/`, `images/`, `data_models/`）
- CSS/SCSS 类名：BEM 命名法（Block__Element--Modifier）
- HTML 文件、JS 文件统一使用短横线命名法

**字节跳动**
- 参考青训营 JS 编码规范：HTML/CSS/JS 各司其职；使用 `const`/`let` 代替 `var`；箭头函数优先
- Go 项目结构遵循 Hertz Kitex 脚手架规范：`internal/model/`, `internal/route/`, `internal/handler/`, `internal/repo/`

**Spring Framework**
- 使用 `spring-javaformat` 插件强制代码风格——不支持自定义配置，"代码适应工具"
- 版权头统一：`Copyright 2002-present the original author or authors.`（Apache License 2.0）
- 使用 JSpecify `@Nullable` 注解（`org.jspecify.annotations.Nullable`）
- 异常类命名以 `Exception` 结尾，构造器提供多种重载（msg、msg+cause）

**Amazon**
- 未提供公开统一的 Java 编码规范，各团队使用 Amazon Q Developer Project Rules 自行管理
- 遵循"Working Backwards"原则：命名直接面向 API/资源，强调资源名使用复数名词

**美团**
- MDP 平台内部规范与阿里规范相近，对外公开内容以技术实践为主（静态代码检查、Lint 规则）
- CatPaw IDE 强调"人人对齐 -> 人机对齐"，核心标准对齐理念落地为可执行约束

---

## 二、代码风格

### 2.1 缩进与行长

| 厂/项目 | 缩进 | 行长限制 | 备注 |
|---------|------|----------|------|
| Google (Java) | 2 空格 | 100 字符 | lambda 箭头不换行断；不允许 Tab |
| Google (C++) | 2 空格 | 80 字符 | 可扩展至 100 |
| Google (Python) | 4 空格（PEP 8 兼容） | 80 字符 | Google Python Style 同 PEP 8 |
| 阿里巴巴 | 4 空格 | 120 字符（IDE 设置） | 单行不超过 150 字符警告 |
| Microsoft C# | 4 空格 | editorconfig 可配 | 无硬性统一数字 |
| Spring | 由 `spring-javaformat` 插件决定 | 不可自定义 | 插件格式化 |
| 腾讯 AlloyTeam | 4 空格（前端） | 120 字符 | |
| Kubernetes Go| `gofmt`/`goimports` 决定 | Go 社区惯例 | Tab 缩进 |

### 2.2 大括号风格

**Kernighan & Ritchie（K&R）风格（Google Java、Microsoft、Spring 通用）**：
```java
if (condition) {
  doSomething();
} else {
  doOther();
}
```
- 左花括号前不换行（函数/类/if/for/while/try 统一）
- 空块可简写为 `{}`，但多块语句（if/else, try/catch）不可简写
- `if`/`else`/`for`/`while`/`do`/`switch`/`try`/`catch`/`finally`/`with` 后**必须有大括号**（即使单行）——腾讯 AlloyTeam、阿里巴巴同时强制

### 2.3 Import 排序

**Google Java Style**：
```
1. 全部静态 import 一组（无通配符）
2. 全部非静态 import 一组
3. 两组之间用空行分隔
4. 各组内按 ASCII 排序
5. 禁止静态导入嵌套类
```

**阿里巴巴**：
- 禁止通配符 import（`import java.util.*`）
- import 顺序：`java` → `javax` → 第三方 → 项目内部
- 分组之间空行分隔

**Spring / Spring Boot**：
- `io.spring.javaformat` + `spring-javaformat-maven-plugin`
- Intellij IDEA 插件 + Eclipse 插件 双端覆盖

### 2.4 空行规范

- Google：类成员之间（字段、构造器、方法）用单个空行分隔；字段之间空行可选
- Google：两个连续空行允许但不鼓励
- 阿里巴巴：方法体内逻辑块之间空行分隔；方法之间无强制空行
- Microsoft：每个 `}` 后空行分隔逻辑块（editorconfig `csharp_before_block_opening`）

### 2.5 注释格式

- Google：使用实现注释（`//`），不使用 `/* */` 做 Javadoc
- 阿里巴巴：类/方法/字段需要 Javadoc，使用 `@author` `@since` `@param` `@return` `@throws`
- Spring：完整的 Javadoc style 注释，所有 public API 必须有 Javadoc；源码类头包含版权信息
- Google C++ Style：使用 Doxygen 格式注释

---

## 三、分层架构

### 3.1 经典三层架构

```
Controller（表现层） → Service（业务逻辑层） → DAO（数据访问层）
```

**核心规则**：
- **单向依赖**：Controller 只能依赖 Service，Service 只能依赖 DAO，反向禁止
- **禁止跨层调用**：Controller 直接调 DAO 严重违反规范（阿里巴巴 P0 级别错误）
- **Service 层不得持有 Request/Response 对象**

### 3.2 阿里巴巴四层架构（《Java开发手册》推荐）

| 层 | 职责 | 命名后缀 |
|----|------|---------|
| Web 层（Controller） | 访问控制转发、基本参数校验、不复用的简单业务 | `XxxController` |
| Service 层 | 核心业务逻辑 | `XxxService` / `XxxServiceImpl` |
| Manager 层 | 通用业务处理层（第三方接口封装、缓存策略、中间件接入） | `XxxManager` |
| DAO 层（Mapper） | 数据访问，与数据库交互 | `XxxMapper` / `XxxDAO` |

### 3.3 DTO / VO / Entity / BO 严格分层

```
Entity    —— 数据库表映射对象（DAO 层产出，仅内部流转）
DTO       —— 数据传输对象（Service/Controller 传入传出）
VO        —— 视图对象（返回给前端/调用方的展示对象，可裁剪字段）
BO        —— 业务对象（Service 层内部使用的领域对象）
```

**阿里巴巴强制**：
- DAO 层严禁返回 `Map`、`List` 等弱类型集合，必须封装为 VO/DTO/BO
- Controller 层禁止直接返回 Entity 实体（防止泄露数据库结构）
- 各层对象转换通过工具类（如 `BeanUtils.copyProperties`）或 MapStruct

### 3.4 Spring 项目标准结构

```
com.example.project
├── Application.java          # 启动类
├── config/                   # 配置类
├── controller/               # REST 控制器
├── service/                  # 业务逻辑接口
│   └── impl/                 # 业务逻辑实现
├── repository/               # 数据访问层（Spring Data）
├── model/                    # 领域模型/entity
├── dto/                      # 数据传输对象
├── vo/                       # 视图对象
├── mapper/                   # 对象转换
├── exception/                # 异常定义
├── aspect/                   # AOP 切面
└── util/                     # 工具类
```

### 3.5 字节跳动 / 美团微服务分层（基于 Hertz/CloudWeGo Kitex）

```
internal/
├── model/        # 数据模型
├── repo/         # 数据访问（repository 模式）
├── handler/      # HTTP/RPC 处理层
├── route/        # 路由定义
├── service/      # 业务逻辑
└── middleware/   # 中间件
pkg/              # 可复出的公共包
```

---

## 四、异常处理

### 4.1 统一异常封装

**阿里巴巴规范**：
- 定义全局异常处理器 `@ControllerAdvice` + `@ExceptionHandler`
- 统一返回 `Result<T>` 结构：
```java
public class Result<T> {
    private Integer code;
    private String message;
    private T data;
    private Long timestamp;
}
```
- 禁止 `catch (Exception e)` 裸捕获后直接吞掉异常
- 异常链必须完整传递：`throw new BusinessException("msg", originalException)`
- RuntimeException 必须有有意义的业务描述信息

**Spring Framework 源码实践**：
- 分层异常体系：`DataAccessException` → `DataAccessResourceFailureException` / `CannotGetJdbcConnectionException`
- 异常链不丢失根因：`public CannotGetJdbcConnectionException(String msg, SQLException ex) { super(msg, ex); }`
- 从不吞异常；所有 checked exception 包装为未检查异常（Spring 哲学）

**Google Java Style**：
- 禁止使用异常控制正常流程
- 方法调用者应捕获可以处理的异常；不可处理的异常向上抛出
- 优先使用标准库异常（`IllegalArgumentException`, `IllegalStateException`, `NullPointerException`）

**Microsoft .NET**：
- `try-catch` 时优先使用不带异常对象的 `catch` 子句（.NET 2.0+ 后不带变量会编译错误）
- 异常筛选器：`catch (Exception ex) when (ex is SqlException sqlEx && sqlEx.Number == 1205)`
- 自定义异常继承 `ApplicationException`（已不推荐）或 `Exception`

### 4.2 错误码规范

**GTTS/HTTP 状态码对应**：

| HTTP 状态码 | 语义 | 业务场景 |
|-------------|------|----------|
| 200 | 成功 | 正常返回 |
| 400 | Bad Request | 参数校验失败 |
| 401 | Unauthorized | 未登录或 token 无效 |
| 403 | Forbidden | 无权限 |
| 404 | Not Found | 资源不存在 |
| 409 | Conflict | 冲突（如重复创建） |
| 422 | Unprocessable Entity | 语义校验失败（字段格式正确但biz逻辑不合理） |
| 429 | Too Many Requests | 限流 |
| 500 | Internal Server Error | 服务端内部错误 |

**错误码体系设计（阿里巴巴）**：
```
A          B         C
│          │         │
错误级别   错误模块   错误序号

示例：
A = 1: 用户端错误
  B = 001: 用户注册错误
    C = 0001: 用户已存在 → 完整码 10010001

A = 2: 系统端错误
  B = 001: 数据库错误  
    C = 0001: 数据库连接失败 → 完整码 20010001
```

---

## 五、日志规范

### 5.1 日志级别使用规则

| 级别 | 使用场景 |
|------|----------|
| ERROR | 系统异常、不可恢复错误、影响核心业务的异常 |
| WARN | 可恢复异常、降级逻辑、即将过期提示、参数可疑 |
| INFO | 关键业务流程节点、服务启动/停止、配置变更、外部接口调用 |
| DEBUG | 调试信息、方法入参出参、分支判断日志 |
| TRACE | 最细粒度信息，生产环境默认关闭 |

### 5.2 占位符使用（核心规则）

**阿里巴巴强制义务**：
```java
// 正确 ✅——SLF4J 占位符
logger.info("用户 {} 登录成功，IP {}", userName, ip);

// ❌ 错误——禁止字符串拼接
logger.info("用户 " + userName + " 登录成功，IP " + ip);

// ❌ 错误——INFO/FATAL 以下级别不打印(DEBUG/TRACE 除外),但 debug 时需要级别判断
// 低级别日志必须加 isDebugEnabled 判断
if (logger.isDebugEnabled()) {
    logger.debug("详情：" + expensiveOperation());
}
```

**字节跳动 EB 级日志系统 TLS（Tinder Log Service）实践**：
- 所有日志统一到 TLS 平台
- 结构化日志：日志即 trace，trace 即日志
- 支持日志转 metrics 实时告警
- 自研倒排索引实现 EB 级快速检索

**Spring Framework**：
- 使用 SLF4J + Logback
- 生产环境默认级别：INFO
- Apache License 2.0 版权头 + 类级 Javadoc

### 5.3 敏感信息脱敏

**必须脱敏字段**（阿里巴巴安全规约）：
- 手机号（中间 4 位打码：`138****1234`）
- 身份证号（出生月日或末 4 位打码）
- 银行卡号（仅显示末 4 位）
- 密码 / Token / Secret Key（完全不输出）
- 地址信息（街道以下打码）
- 邮箱（用户名保留首位 + `***`）

**做法**：
- 实现自定义 `MessageConverter` 或 `JsonSerializer` 实现全局脱敏
- 基于注解 `@Sensitive(field = SensitiveType.PHONE)`

### 5.4 结构化日志（业界前沿）

```json
{
  "timestamp": "2025-01-15T10:30:00.000Z",
  "level": "INFO",
  "traceId": "abc123",
  "spanId": "def456",
  "service": "order-service",
  "method": "OrderService.createOrder",
  "userId": "10086",
  "duration_ms": 45,
  "message": "订单创建成功"
}
```

---

## 六、安全规范

### 6.1 参数校验

**强制（阿里巴巴）**：
- Controller 层使用 `@Valid` + `@NotNull` `@NotBlank` `@Size` `@Min` `@Max` `@Pattern` 注解
- Service 层方法对关键参数做 `if (param == null) throw new IllegalArgumentException()`
- 数据库字段长度约束：`varchar(64)` 不要开到 `varchar(255)` 无脑

### 6.2 SQL 注入防护

- **强制使用预编译（PreparedStatement）**，禁止字符串拼接 SQL
- MyBatis 使用 `#{}` 而非 `${}`
- ORDER BY 字段不可直接拼接，使用白名单校验
- 阿里巴巴安全规约：执行 SQL 时需通过安全工具（如 Druid SQL Wall）

### 6.3 XSS 防护

- 前端输入输出全部做 HTML 转义
- 富文本使用白名单过滤（HTMLPurifier 等）
- CSP（Content Security Policy）HTTP 头配置
- 阿里巴巴： HttpServletResponse 设置正确的 Content-Type 和 Charset

### 6.4 接口限流

**主流方案对比**：

| 方案 | 实现 | 适用场景 |
|------|------|----------|
| 令牌桶 | Sentinel / Guava RateLimiter | API 网关限流 |
| 漏桶 | NginxLimitReq 模块 | 入口层限流 |
| 计数器 | Redis + Lua | 分布式限流 |
| 熔断 | Sentinel / Hystrix / Resilience4j | 下游保护 |

**阿里巴巴 Sentinel 实践**：
- 流控规则（QPS/线程数）
- 熔断降级规则（慢调用比例/异常比例/异常数）
- 热点参数限流（同一 ID 调用频率限制）

### 6.5 数据脱敏

- DTO/VO 层脱敏：全局 Jackson 序列化器控制
- 数据库层加密：手机号、身份证使用 AES 加密存储，提供解密接口
- 日志输出脱敏（见 5.3）

---

## 七、并发规范

### 7.1 线程池使用（阿里巴巴强制规则）

**必须通过 `ThreadPoolExecutor` 显式创建**：
```java
// ❌ 禁止使用——有 OOM 风险
ExecutorService pool = Executors.newFixedThreadPool(10);
ExecutorService pool = Executors.newCachedThreadPool();

// ✅ 正确——显式指定所有参数
ThreadPoolExecutor executor = new ThreadPoolExecutor(
    5,                      // corePoolSize
    10,                     // maximumPoolSize
    60L,                    // keepAliveTime
    TimeUnit.SECONDS,
    new LinkedBlockingQueue<>(1000),   // 有界队列
    new ThreadFactoryBuilder().setNameFormat("order-thread-%d").build(),
    new ThreadPoolExecutor.CallerRunsPolicy()  // 拒绝策略
);
```

**线程池命名规范**：使用自定义 `ThreadFactory`，名称包含业务域：
```
trade-pay-thread-pool
order-async-thread-pool
```

**Google Java**：
- 使用 `Executors` 快捷方法可以接受（Google Guava 提供 `ThreadFactoryBuilder`）
- 推荐使用 Guava 的 `RateLimiter` 做限流
- 线程安全集合用 `ConcurrentHashMap` `CopyOnWriteArrayList`

**Microsoft .NET**：
- 使用 `ThreadPool.QueueUserWorkItem` 或 `Task.Run`
- 异步方法命名以 `Async` 结尾
- ConfigureAwait(false) 在库代码中强制使用
- 使用 `SemaphoreSlim` 做异步限流

### 7.2 锁粒度

- **最小化锁范围**：只锁必要代码块
- **优先使用并发工具类**：`ConcurrentHashMap`, `AtomicLong`, `CountDownLatch`, `CyclicBarrier`
- **避免锁嵌套**：防止死锁
- `volatile` 仅限状态标志位，不可替代 `synchronized`（阿里巴巴强制）

### 7.3 线程安全集合

| 用途 | 推荐类 | 替代 |
|------|--------|------|
| 并发 Map | `ConcurrentHashMap` | Hashtable / Collections.synchronizedMap |
| 并发 List | `CopyOnWriteArrayList` (读多写少) | synchronizedList |
| 并发 Set | `ConcurrentHashMap.newKeySet()` | synchronizedSet |
| 阻塞队列 | `LinkedBlockingQueue` / `ArrayBlockingQueue` | 手写 wait/notify |
| 原子变量 | `AtomicLong`, `AtomicInteger` | synchronized 包装 int |

---

## 八、数据库规范

### 8.1 表命名

- 全小写，下划线分隔：`t_order`, `t_order_item`
- 表名使用复数或单数形式——团队内统一（推荐单数：`order`）
- 必须包含 `create_time`, `update_time` 字段（阿里巴巴强制）
- 主键统一 `bigint` 类型：`id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY`
- 软删除：`is_deleted TINYINT(1) NOT NULL DEFAULT 0`
- 表注释 `COMMENT`，所有列必须有注释

### 8.2 索引规范（阿里巴巴强制）

- 唯一索引命名：`uniq_字段1_字段2`
- 普通索引命名：`idx_字段1_字段2`
- 复合索引字段顺序：区分度高的在前，等值查询字段在前
- 覆盖索引：查询列尽量在索引中，减少回表
- 避免在 WHERE 子句中对字段进行函数操作：`WHERE DATE(create_time) = '2025-01-15'` ❌

### 8.3 ORM 映射

**MyBatis Mapper 层规范**：
- XML mapper 与接口一一对应
- 复杂 SQL 写在 XML 中，简单 SQL 可用注解
- 动态 SQL 使用 `<if>`, `<choose>`, `<foreach>`，避免字符串拼接
- 阿里巴巴：DAO 层接口方法命名：`selectXxx` / `insertXxx` / `updateXxx` / `deleteXxx`

### 8.4 分页查询

- 物理分页（推荐）：`LIMIT offset, size`
- 深分页优化：`WHERE id > lastId LIMIT 20`（游标分页）
- 禁止无 LIMIT 的全表扫描查询
- 阿里巴巴：分页查询结果封装为 `PageResult<T>`,含 `total`, `list`, `pageNum`, `pageSize`

### 8.5 事务管理

- `@Transactional` 加在 Service 层，不在 DAO 层
- 明确指定 `propagation` 和 `isolation`
- 非事务方法调用事务方法不生效（Spring AOP 自调用问题）
- 阿里巴巴：事务范围尽量小，避免 RPC 调用放在事务内
- 阿里巴巴：数据变更时使用 `@Version` 乐观锁，避免死锁

---

## 九、API 设计规范

### 9.1 RESTful 约定

**Google API Design Guide（业界权威）**：
- 面向资源设计：URL 只包含名词，不包含动词
- 资源使用复数名词：`/orders`, `/users`
- 标准方法映射：

| HTTP 方法 | 语义 | 幂等 |
|-----------|------|------|
| GET | 查询资源 | 是 |
| POST | 创建资源 | 否 |
| PUT | 替换资源 | 是 |
| PATCH | 部分更新 | 否 |
| DELETE | 删除资源 | 是 |

- 动作转化为子资源：`POST /orders/{id}/refund`
- 资源层级：`/users/{userId}/orders`
- URL 长度一般不超过 2048 字符

**阿里巴巴**：
- URL 设计风格与 Google 类似
- 建议使用 `/api/v1/` 前缀做版本管理
- 查询参数：`?page=0&size=20&sort=createTime,desc`

### 9.2 API 版本控制

| 策略 | 做法 | 特点 |
|------|------|------|
| URI 版本 | `/v1/orders` | 直观，最常用，易缓存 |
| 头版本 | `Accept: application/vnd.api.v2+json` | 更"纯" REST，但不直观 |
| 参数版本 | `?version=2` | 少用，不推荐 |

**兼容性纪律**：
- 加字段向后兼容
- 删字段、改类型、改语义必须升版本
- 旧版本保留过渡期后下线

### 9.3 响应格式标准

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 123,
    "name": "订单名称"
  },
  "traceId": "abc123def456",
  "timestamp": 1705286400000
}
```

**错误响应**：
```json
{
  "code": 40001,
  "message": "用户ID不能为空",
  "errors": [
    {
      "field": "userId",
      "code": "NotNull",
      "message": "用户ID不能为空"
    }
  ],
  "traceId": "abc123def456"
}
```

### 9.4 gRPC API 设计

**Google gRPC 标准**：
- 错误使用 `google.rpc.Status` 和 `google.rpc.Code`
- 错误详情使用 `google.rpc.ErrorInfo`, `google.rpc.BadRequest`, `google.rpc.PreconditionFailure` 等
- 错误码优先级：`OUT_OF_RANGE` > `FAILED_PRECONDITION`；`NOT_FOUND` / `ALREADY_EXISTS` > `FAILED_PRECONDITION`

---

## 十、测试规范

### 10.1 测试覆盖率要求

| 厂/项目 | 行覆盖率 | 分支覆盖率 | 说明 |
|---------|----------|------------|------|
| Google | ≥ 80% | 鼓励 | 不设硬性数字，按需覆盖核心路径 |
| Microsoft | ≥ 80% | ≥ 70% | editorconfig + Coverlet 集成 |
| Alibaba | ≥ 80%（核心模块） | 鼓励 | 单元测试为 CI 门禁 |
| Spring | 核心模块 ≥ 90% | 高 | JaCoCo 强制 |

### 10.2 测试命名

**业界通用模式——Given-When-Then**：
```java
// 模式1：方法名_预期行为_测试场景
@Test
void createOrder_throwsException_whenProductStockInsufficient() { }

// 模式2：Given-When-Then 三段式
@Test
void givenNewOrder_whenSubmit_thenStatusIsCreated() { }
```

**阿里巴巴推荐**：
- 测试类命名：`XxxServiceTest` / `XxxServiceImplTest`
- 方法命名：`testMethodName` 前缀（JUnit 4）或直接描述行为（JUnit 5）

### 10.3 Mock 策略

- **Google**：使用 Mockito 2 / jMock；优先依赖注入方便测试
- **Spring Boot**：使用 `@MockBean` 做 Spring 上下文 Mock
- **阿里巴巴**：核心业务逻辑 Mock 外部依赖（DAO、第三方接口），不 Mock 外部框架
- **原则**：
  - Mock 应该只用于外部依赖和基础设施
  - 不 Mock 值对象和 DTO
  - Stub 行为优先 over 行为验证

---

## 十一、Git 规范

### 11.1 分支命名

| 厂/项目 | 命名格式 |
|---------|----------|
| Google | `feature/xxx`, `bugfix/xxx`, `release/x.y.z` |
| 阿里巴巴 | `feature/JIRA-1234-add-user-feature`, `bugfix/JIRA-1234-fix-null-pointer` |
| 腾讯 AlloyTeam | `feature/xxx`, `bugfix/xxx`, `hotfix/xxx`, `release/xxx` |
| Spring | `issue/gh-{issue-number}`, `ticket/JIRA-{number}` |
| Kubernetes | `feature/{descriptive-name}`, `fix/{descriptive-name}` |

### 11.2 Commit Message（Conventional Commits 通用）

```
<type>(<scope>): <subject>

<body>

<footer>
```

**type 列表**：
- `feat`: 新功能
- `fix`: 修复 bug
- `refactor`: 重构
- `perf`: 性能优化
- `docs`: 文档更新
- `test`: 测试相关
- `chore`: 构建/工具变动
- `style`: 代码格式变动（不影响逻辑）
- `ci`: CI 配置变更

**scope**：模块名（`feat(order):`、`fix(payment):`）
**subject**：简洁描述，不超过 50 字符，现在时态，祈使句
**body**：变更细节、动机、影响
**footer**：关联 `Closes #123`, `Co-authored-by:`

**Google 额外规则**：
- 每行不超过 72 字符
- 使用现在时态："Add feature" not "Added feature"
- 使用祈使语态："Moves cursor to..." not "Moves cursor to..."

### 11.3 PR Checklist（通用模板）

```
- [ ] 代码风格符合项目规范
- [ ] 单元测试通过，覆盖率 ≥ 80%
- [ ] 新增功能已添加集成测试
- [ ] 接口变更已更新文档
- [ ] 数据库变更脚本已提供
- [ ] 依赖更新已说明原因和兼容性
- [ ] 敏感信息已脱敏
- [ ] 异常处理完善，错误码有说明
- [ ] 日志级别和内容适当
- [ ] 性能影响已评估
```

**Kubernetes PR 强制**：
- 关联 `Fixes #issue-number`
- PR 描述填写动机、变更说明、测试方法
- CNCF CLA 签署
- `/ok-to-test` 标签触发自动化测试

---

## 十二、依赖管理

### 12.1 版本锁定

**Maven (Spring Boot / Alibaba)**：
- 使用 `dependencyManagement` 统一管理依赖版本
- Spring Boot BOM 模式：只需指定 Spring Boot 版本，框架自动管理所有兼容版本
- 禁止在子模块中硬编码版本号
- `flatten-maven-plugin` 处理 `${revision}` CI-Friendly 版本

**Gradle (Google)**：
- 使用 `constraints` 块管理版本约束
- 使用 Platform（`spring-boot-dependencies` 或自创 Platform）
- Gradle Version Catalog 统一管理（`libs.versions.toml`）

**NPM / Node.js（腾讯 AlloyTeam / 前端）**：
- 使用 `package-lock.json` 或 `yarn.lock` 锁定依赖
- 核心依赖使用精确版本（`1.2.3`），非核心依赖使用 `^1.2.3`
- `.npmrc` 配置 registry mirror（美团内部使用内部源）

### 12.2 依赖审查规则

**阿里巴巴安全规约**：
- 定期使用 `mvn dependency:tree` 审查依赖树
- 禁止使用 SNAPSHOT 版本上生产
- 依赖版本升级需全量回归测试
- 使用安全工具扫描已知漏洞（Snyk / OWASP Dependency-Check）

**Spring Framework**：
- 通过 `spring-javaformat-checkstyle` 和 Maven 插件强制规范
- 不允许 SNAPSHOT 依赖上主分支
- `999-SNAPSHOT` 用于开发期，发布时替换精确版本

### 12.3 三方库选型原则

| 原则 | 说明 |
|------|------|
| 社区活跃度 | GitHub Star + 最近一年有 Release |
| License 兼容 | Apache 2.0 / MIT 优先；GPL 系列禁止用于商业闭源 |
| 安全性 | CVE 历史、安全团队响应速度 |
| 维护状态 | 核心维护者 ≥ 2 人，Bus Factor ≥ 2 |
| 替代性 | 是否为公司/Spring 官方提供（优先 Spring 官方方案） |
| 包体积 | 评估对构建产物和启动时间的影响 |
| 文档化 | README 完整、有 CHANGELOG、Migration Guide |

**具体推荐（Java 生态通用）**：
- Web 框架：Spring Boot（必选首选）
- ORM：MyBatis-Plus / JPA（视团队习惯）
- 连接池：HikariCP（Spring Boot 默认）
- JSON：Jackson（Spring 默认）
- 工具库：Lombok / Hutool / Guava
- 限流熔断：Sentinel
- 单元测试：JUnit 5 + Mockito + AssertJ
- 日志：SLF4J + Logback / Log4j2

---

## 十三、自动化落地工具链建议

### 13.1 编码规范自动化（按 Java 生态推荐）

```
                    ┌─────────────────────────┐
                    │   本地开发阶段           │
                    │                         │
                    │  IDE Checkstyle/        │
                    │  SonarLint 插件          │
                    │  spring-javaformat-     │
                    │  eclipse-plugin          │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  编译阶段                │
                    │                         │
                    │  maven-checkstyle-plugin│
                    │  maven-pmd-plugin       │
                    │  spring-javaformat-     │
                    │  maven-plugin           │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  CI/CD 门禁             │
                    │                         │
                    │  SonarQube / SonarCloud │
                    │  分支覆盖率门禁          │
                    │  依赖安全扫描            │
                    │  单元测试通过率          │
                    └─────────────────────────┘
```

### 13.2 具体工具推荐

| 环节 | 工具 | 作用 |
|------|------|------|
| 代码格式化 | `spring-javaformat-maven-plugin` / Google Java Format | 自动格式化 |
| 静态检查 | Checkstyle + Sun Checks / Google Checks | 风格+命名 |
| 代码质量 | SonarQube（9.9+ LTS） | 复杂度+覆盖率+Bug |
| 安全扫描 | OWASP Dependency-Check / Snyk | 依赖漏洞 |
| 单元测试 | JUnit 5 + Mockito + JaCoCo | 测试+覆盖率 |
| Git Hook | Husky / pre-commit | Commit 前检查 |
| CI 质量门禁 | GitHub Actions / Jenkins | PR 准入条件 |

---

## 附录：主要参考规范全文

| 规范 | 来源 | 地址 |
|------|------|------|
| 阿里巴巴 Java 开发手册（嵩山版 v1.5.0） | 阿里巴巴集团 | https://github.com/alibaba/p3c |
| Google Java Style Guide | Google | https://google.github.io/styleguide/javaguide.html |
| Google API Design Guide | Google | https://cloud.google.com/apis/design |
| Google C++ Style Guide | Google | https://google.github.io/styleguide/cppguide.html |
| C# Coding Conventions | Microsoft | https://learn.microsoft.com/dotnet/csharp/fundamentals/coding-style |
| .NET Runtime Coding Style | Microsoft | https://github.com/dotnet/runtime/blob/main/docs/coding-guidelines/coding-style.md |
| Spring Framework Conventions | Spring | `spring-javaformat` + 源码自解释 |
| Kubernetes Contributor Guide | CNCF | https://github.com/kubernetes/community/tree/master/contributors/guide |
| 腾讯 AlloyTeam 前端规范 | 腾讯 AlloyTeam | http://alloyteam.github.io/CodeGuide/ |
| ByteDance Hertz | 字节跳动 | https://www.cloudwego.io/docs/hertz/ |
| PEP 8 (Python) | Python.org | https://pep8.org |

---

> **结语**：编码规范的本质不是限制创造力，而是降低团队协作成本。建议结合自身团队规模和业务特点，在 Google / Alibaba / Spring 三大权威规范基础上裁剪适配，并通过自动化工具（IDE 插件 + CI 约束）确保规范的落地执行。针对 AI Coding 时代，应将规范"转译为 AI 可执行的约束"，实现"人人对齐 → 人机对齐"的治理升级。
