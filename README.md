# Passwall Preferred IP Service

一个给 Passwall 用的“优选 IP 订阅服务”。

它做三件事：
- 定时测速，维护优选 IP 池（`preferred_ips.csv`）。
- 把优选 IP 替换进你的订阅模板。
- 提供 HTTP 订阅接口（可输出 Base64）。

## 重要注意事项

- 这是“优选 IP”服务，建议部署在用户本地网络环境（本地电脑、家宽旁路由、家庭服务器）运行。
- 测速时请关闭代理，或在没有开启代理的机器上运行；否则测速流量可能走代理，导致优选结果不准确、不可用。

## 30 秒上手

### 1) 准备配置

使用项目里的 `passwall_config.json` 和 `passwall_template.txt` 即可。

最关键的配置是：
- `enable_subscription`: 是否开启订阅接口（通常 `true`）
- `enable_speedtest`: 是否开启定时测速（调试时可设为 `false`）
- `subscriptions[].token_template_files`: token 到模板文件映射（推荐）
- `subscriptions[].template_file`: 默认模板文件（可选，仅在非 token-only 模式下使用）
- `subscriptions[].base64`: Passwall 常见场景设为 `true`

### 2) 启动

```bash
go run . -config passwall_config.json
```

或：

```bash
go build -o pwip
./pwip -config passwall_config.json
```

### 3) 验证

```bash
curl http://127.0.0.1:8080/healthz
curl 'http://127.0.0.1:8080/sub/passwall?x-token=your-token'
```

如果你配置了 `base64: true`，可这样看明文：

```bash
curl -s 'http://127.0.0.1:8080/sub/passwall?x-token=your-token' | base64 -d
```

## 常用接口

- 健康检查：`GET /healthz`
- 默认订阅：`GET /sub/passwall`（仅在未配置 `token_template_files` 时可用）
- Token 订阅：`GET /sub/passwall?x-token=你的token`

## 模板怎么写

模板支持多行（通常一行一个节点），支持占位符：

- `{{IP}}`：替换为单个优选 IP
- `{{IP_LIST}}`：替换为前 N 个优选 IP（由 `use_top_n` 控制）

示例（多行）：

```txt
vless://uuid@{{IP}}:443?type=ws&security=tls&sni=example.com#node-1
vless://uuid@{{IP}}:443?type=ws&security=tls&sni=example.com#node-2
```

## 常见场景

### 场景 1：只做订阅，不测速（调试）

```json
{
  "enable_subscription": true,
  "enable_speedtest": false
}
```

### 场景 2：订阅按 token 输出不同模板

```json
{
  "subscriptions": [
    {
      "path": "/sub/passwall",
      "token_template_files": {
        "tokenA": "passwall_template_a.txt",
        "tokenB": "passwall_template_b.txt"
      }
    }
  ]
}
```

规则：
- 带 `x-token` 且命中映射：使用对应模板。
- 当配置了 `token_template_files` 时：不带 `x-token` 或 token 未命中都返回 `404`。
- 仅当未配置 `token_template_files` 时，才使用默认 `template_file`。
- 你可以只配置 `token_template_files`（不配置 `template_file/template`），即 token-only 模式。

## 进阶配置（测速）

`speedtest` 与 CFST 常用参数一一对应，常改这几个：

- `routines`: 并发（类似 `-n`）
- `ping_times`: 探测次数（类似 `-t`）
- `test_count`: 下载测速目标数量（类似 `-dn`）
- `download_time_seconds`: 单 IP 下载测速时长（类似 `-dt`）
- `url`: 测速地址（建议你自己的稳定地址）
- `max_delay_ms / max_loss_rate / min_speed_mb`: 筛选条件

## 热更新说明

- 服务按 `config_reload_interval` 定时检查配置文件变化。
- 大部分配置可热更新。
- `listen` 监听地址变更需重启服务。

## 输出文件

默认优选 IP 文件：`preferred_ips.csv`

字段：
- `IP 地址`
- `平均延迟(ms)`
- `下载速度(MB/s)`
- `丢包率`
- `地区码`
- `更新时间`
- `更新次数`

## 版本

```bash
go run . -v
```

## 致谢与声明

本项目基于 [XIU2/CloudflareSpeedTest](https://github.com/XIU2/CloudflareSpeedTest) 代码进行二次开发与功能改造，特此说明并感谢原作者的开源贡献。

## 法律与合规声明

本项目仅供学习、研究与合法测试用途。任何使用者在使用本项目时，必须遵守所在国家或地区的法律法规及相关平台/服务条款。严禁将本项目用于任何非法用途，包括但不限于未授权访问、攻击、滥用网络资源或其他违法行为。因使用者不当使用本项目产生的一切法律责任与后果，由使用者自行承担。
