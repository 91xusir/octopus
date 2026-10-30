<div align="center">

<img src="web/public/logo.svg" alt="Octopus Logo" width="100">

### Octopus

[bestruirui/octopus](https://github.com/bestruirui/octopus) 的个人修改版(基于上游 v0.13.13)。

</div>

## 相对原项目的修改

- **人民币显示**:新增美元兑人民币汇率设置,金额可按人民币显示;模型价格支持按人民币输入,保存时自动换算为美元。数据仍以美元存储,人民币只是显示层。
- **Ollama 渠道**:新增 Ollama 预设,快速接入本地 Ollama 服务(免密钥)。
- **仓库迁移**:Go module 与作者信息改为 `91xusir`,程序内"检查更新"指向本仓库的 [Releases](https://github.com/91xusir/octopus/releases)。
- **移除 Docker 发布**:只提供多平台二进制下载。

## 使用

从 [Releases](https://github.com/91xusir/octopus/releases) 下载对应平台的压缩包,解压后运行:

```bash
./octopus start
```

访问 http://localhost:8080(默认账户 `admin` / `admin`,登录后请立即改密码)。

安装、配置、渠道与分组等功能说明,见[原项目 README](https://github.com/bestruirui/octopus/blob/master/README_zh.md)。

## 更新日志

见 [Releases](https://github.com/91xusir/octopus/releases) 页面,每次发版自动生成。
