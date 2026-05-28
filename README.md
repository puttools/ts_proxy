<<<<<<< HEAD
# ts_proxy
proxy on service request  to preson host
=======
# ts

`ts` 是一个基于 Go 标准库实现的反向隧道代理，包含两个程序：

- `ts-server`：公网侧，监听隧道端口和业务端口
- `ts-client`：内网侧，主动连到 `ts-server`，把流量转发给本地服务

## 构建

```bash
go build -o ts-server ./server
go build -o ts-client ./client
```

## 运行

```bash
# 服务端（自动生成自签名证书）
./ts-server --tunnel-port 9000 --http-port 9001

# 服务端（使用配置文件 server_config.json）
# 示例 server_config.json:
# {
#   "tunnel_port": "9000",
#   "http_port": "9001",
#   "cert_file": "",
#   "key_file": "",
#   "timeout_sec": 60,
#   "log_level": "info",
#   "password": "s3cr3t"
# }
./ts-server --config server_config.json

# 服务端（使用密码认证，从 config.json 读取，或用 --auth-password 覆盖）
# config.json 示例： { "password": "s3cr3t" }
./ts-server --tunnel-port 9000 --http-port 9001 --config config.json

# 客户端（开发模式，跳过证书校验）
./ts-client --server 127.0.0.1:9000 --local 127.0.0.1:8080 --skip-verify

# 客户端 使用密码（或在 client_config.json 中设置 password）
./ts-client --server 127.0.0.1:9000 --local 127.0.0.1:8080 --password s3cr3t

# 客户端（使用配置文件 client_config.json）
# 示例 client_config.json:
# {
#   "server": "1.2.3.4:9000",
#   "local": "127.0.0.1:8080",
#   "ca_file": "",
#   "skip_verify": true,
#   "heartbeat_sec": 30,
#   "log_level": "info",
#   "password": "s3cr3t"
# }
./ts-client --config client_config.json
```

## 测试

```bash
go test ./...
```
>>>>>>> master
