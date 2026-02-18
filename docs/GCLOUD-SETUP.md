# GCloud Kubernetes 集群配置指南

本文档说明如何将 KubeMinds Operator 连接到 GCloud 上的 Kubernetes 集群。

## 📋 场景对比

| 场景 | 集群位置 | 认证方式 | 配置复杂度 | 何时使用 |
|------|---------|--------|----------|---------|
| **GKE (托管)** | GCloud Managed | gcloud CLI | ⭐ 简单 | 企业环保境 |
| **自建 K8s (GCE VM)** | GCloud VM | kubeconfig + SSH | ⭐⭐ 中等 | 成本控制/测试 |
| **本地 Kind** | 本地 | 自带 | ⭐ 简单 | 开发调试 |

---

## 🎯 场景 1: GKE (Google Kubernetes Engine) - 推荐

### 1.1 前置条件

```bash
# 安装 gcloud CLI
curl https://sdk.cloud.google.com | bash

# 初始化 gcloud
gcloud init
gcloud auth login

# 安装 kubectl
gcloud components install kubectl

# 验证集群存在
gcloud container clusters list --project=YOUR_PROJECT_ID
```

### 1.2 获取 kubeconfig

```bash
# 方式 A: 自动配置 (推荐)
gcloud container clusters get-credentials my-cluster \
  --zone us-central1-a \
  --project my-project

# 这会自动修改 ~/.kube/config，添加 GKE 集群信息
# Operator 可以直接使用，无需额外配置

# 验证连接
kubectl cluster-info
kubectl get nodes
```

### 1.3 Operator 配置

#### 选项 A: 使用环境变量 (最简单)

```bash
# Operator 会自动发现 ~/.kube/config
./bin/kubeminds-manager

# 或明确指定
export KUBECONFIG=~/.kube/config
./bin/kubeminds-manager
```

#### 选项 B: 使用 config.yaml

```yaml
# config.yaml
k8s:
  provider: ""              # 空值 = 自动发现 ~/.kube/config
  # 或显式指定:
  # provider: "gcloud"
  # kubeconfigPath: "~/.kube/config"
  insecureSkipVerify: false # GKE 不需要禁用 TLS
  context: ""               # 可选，如有多个集群
```

#### 选项 C: 使用命令行标志

```bash
./bin/kubeminds-manager \
  --k8s-provider=gcloud \
  --kubeconfig-path=~/.kube/config
```

### 1.4 验证部署

```bash
# 创建 namespace
kubectl create namespace kubeminds-system

# 安装 CRD
kubectl apply -f config/crd/bases/

# 启动 Operator
nohup ./bin/kubeminds-manager > operator.log 2>&1 &

# 验证
kubectl get deployments -n kubeminds-system
kubectl logs -f deployment/kubeminds-manager -n kubeminds-system
```

---

## 🎯 场景 2: 自建 K8s on GCE VM (本次实际采用)

这是你目前的环境：K8s 部署在 GCloud VM 上，使用自签名证书。

### 2.1 我的实现方式 (SSH + 手动配置)

```bash
# Step 1: 通过 gcloud SSH 连接到 VM
gcloud compute ssh instance-name --zone asia-east1-b

# Step 2: 在 VM 上获取 kubeconfig (kubeadm 部署)
sudo cat /etc/kubernetes/admin.conf > kubeconfig

# Step 3: 下载到本地
exit  # 退出 SSH
gcloud compute scp instance-name:~/kubeconfig ~/.kube/config \
  --zone asia-east1-b

# Step 4: 修改服务器地址 (关键!)
# vim ~/.kube/config
# 找到这一行:
#   server: https://10.140.0.2:6443  (内部 IP)
# 改为:
#   server: https://35.236.172.169:6443  (外部 IP)

# Step 5: 禁用 TLS 验证 (因为证书不匹配)
# kubectl config set-cluster kubernetes --insecure-skip-tls-verify=true

# Step 6: 验证
kubectl cluster-info
kubectl get nodes
```

### 2.2 Operator 配置 (推荐方式)

#### config.yaml 配置

```yaml
# cmd/config/config.yaml
k8s:
  provider: "gcloud"
  kubeconfigPath: "~/.kube/config"  # 修改后的 kubeconfig
  insecureSkipVerify: true           # 禁用 TLS (测试环境)
  context: "kubernetes-admin@kubernetes"  # kubeadm 默认 context
```

#### 命令行启动

```bash
./bin/kubeminds-manager \
  --k8s-provider=gcloud \
  --kubeconfig-path=~/.kube/config \
  --insecure-skip-tls-verify=true \
  --mock-llm  # 先用 mock 测试
```

---

## 🔧 如何用 SDK 封装改进

当前 `internal/config/k8s.go` 的实现比较基础。这里是改进方案：

### 3.1 当前实现的问题

```go
// internal/config/k8s.go (当前)
case K8sProviderGCloud:
    return buildFromKubeconfig(cfg.K8s.KubeconfigPath, cfg.K8s.Context, cfg.K8s.InsecureSkipVerify)
    // ❌ 只是简单代理，无 GCloud 特定逻辑
```

### 3.2 改进方案 A: 使用 GCloud SDK 自动配置

```go
// internal/config/gcloud.go (新增)
package config

import (
    "context"
    "fmt"
    "os"

    "cloud.google.com/go/container/apiv1"
    "google.golang.org/api/option"
    "k8s.io/client-go/rest"
    "k8s.io/client-go/tools/clientcmd"
)

// GCloudProvider handles GCloud GKE cluster authentication
type GCloudProvider struct {
    Project   string // GCP Project ID
    Zone      string // GKE Zone
    Cluster   string // GKE Cluster Name
}

// GetConfig returns a *rest.Config for GKE cluster
func (g *GCloudProvider) GetConfig(ctx context.Context) (*rest.Config, error) {
    // Step 1: 使用 gcloud SDK 获取集群信息
    client, err := container.NewClusterManagerClient(ctx, option.WithScopes(
        "https://www.googleapis.com/auth/cloud-platform",
    ))
    if err != nil {
        return nil, fmt.Errorf("failed to create GCloud client: %w", err)
    }
    defer client.Close()

    // Step 2: 获取 GKE 集群数据
    clusterPath := fmt.Sprintf("projects/%s/zones/%s/clusters/%s",
        g.Project, g.Zone, g.Cluster)

    cluster, err := client.GetCluster(ctx, &containerpb.GetClusterRequest{
        Name: clusterPath,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to get GKE cluster: %w", err)
    }

    // Step 3: 构建 rest.Config
    certData := cluster.MasterAuth.ClusterCaCertificate
    config := &rest.Config{
        Host:     fmt.Sprintf("https://%s", cluster.Endpoint),
        CAData:   []byte(certData),
        TLSClientConfig: rest.TLSClientConfig{
            Insecure: false,
        },
    }

    // Step 4: 添加认证 (使用 gcloud auth)
    // 这部分需要集成 gcloud 令牌认证
    // ...

    return config, nil
}

// NewK8sRestConfig in config.go
func NewK8sRestConfig(cfg *Config) (*rest.Config, error) {
    switch cfg.K8s.Provider {
    case K8sProviderGCloud:
        // 如果有完整的 GKE 参数，使用 SDK 方式
        if cfg.K8s.GCloud != nil {
            provider := &GCloudProvider{
                Project: cfg.K8s.GCloud.Project,
                Zone:    cfg.K8s.GCloud.Zone,
                Cluster: cfg.K8s.GCloud.Cluster,
            }
            return provider.GetConfig(context.Background())
        }
        // 否则回退到 kubeconfig 方式
        return buildFromKubeconfig(cfg.K8s.KubeconfigPath, cfg.K8s.Context, cfg.K8s.InsecureSkipVerify)

    case K8sProviderLocal:
        return buildFromKubeconfig(cfg.K8s.KubeconfigPath, cfg.K8s.Context, false)

    default:
        return ctrl.GetConfigOrDie(), nil
    }
}
```

### 3.3 改进方案 B: SSH 隧道方式 (当前场景最优)

对于自建在 GCE VM 上的 K8s，推荐用 SSH 隧道：

```go
// internal/config/ssh_tunnel.go (新增)
package config

import (
    "fmt"
    "net"
    "os/exec"

    "k8s.io/client-go/rest"
    "k8s.io/client-go/tools/clientcmd"
)

// CreateSSHTunnel creates a port-forward tunnel to K8s API via SSH
func CreateSSHTunnel(vmInstance, zone, targetPort string) (string, error) {
    // 启动 gcloud compute ssh 隧道
    localPort := "16443"  // 本地端口
    remotePort := targetPort  // 远程 K8s API 端口 (通常 6443)

    cmd := exec.Command("gcloud", "compute", "ssh",
        vmInstance,
        "--zone", zone,
        "--ssh-flag=-L", fmt.Sprintf("%s:localhost:%s", localPort, remotePort),
    )

    // 注意: 这会阻塞，需要在后台运行
    // 实际使用时需要更复杂的生命周期管理

    return fmt.Sprintf("https://localhost:%s", localPort), nil
}

// NewK8sRestConfigWithSSHTunnel creates a rest.Config via SSH tunnel
func NewK8sRestConfigWithSSHTunnel(cfg *Config) (*rest.Config, error) {
    gcloudCfg := cfg.K8s.GCloud

    // Step 1: 建立 SSH 隧道
    localEndpoint, err := CreateSSHTunnel(
        gcloudCfg.Instance,
        gcloudCfg.Zone,
        "6443",
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create SSH tunnel: %w", err)
    }

    // Step 2: 修改 kubeconfig 中的服务器地址
    restConfig, err := buildFromKubeconfig(
        cfg.K8s.KubeconfigPath,
        cfg.K8s.Context,
        false, // TLS 验证开启
    )
    if err != nil {
        return nil, err
    }

    // Step 3: 覆盖 Host
    restConfig.Host = localEndpoint

    return restConfig, nil
}
```

### 3.4 改进的 Config 结构

```go
// internal/config/config.go
type K8sConfig struct {
    Provider            K8sProvider `yaml:"provider"`
    KubeconfigPath      string      `yaml:"kubeconfigPath"`
    Context             string      `yaml:"context"`
    InsecureSkipVerify  bool        `yaml:"insecureSkipVerify"`

    // GCloud 专属配置
    GCloud *GCloudConfig `yaml:"gcloud"`
}

type GCloudConfig struct {
    // GKE 模式
    Project string `yaml:"project"`    // GCP Project ID
    Zone    string `yaml:"zone"`       // GKE Zone
    Cluster string `yaml:"cluster"`    // GKE Cluster Name

    // 自建 K8s on GCE 模式
    Instance string `yaml:"instance"`  // GCE VM Instance Name
    SSHTunnel bool   `yaml:"sshTunnel"` // 是否使用 SSH 隧道
}
```

### 3.5 改进后的 config.yaml 示例

```yaml
# GKE 模式
k8s:
  provider: "gcloud"
  gcloud:
    project: "my-project"
    zone: "us-central1-a"
    cluster: "my-gke-cluster"

# 或自建 K8s on GCE 模式
k8s:
  provider: "gcloud"
  kubeconfigPath: "~/.kube/config"
  insecureSkipVerify: true  # 自签名证书
  gcloud:
    instance: "instance-20260215-051955"
    zone: "asia-east1-b"
    sshTunnel: true  # 可选: 使用 SSH 隧道而不是直接连接
```

---

## 🔄 推荐的分阶段实现

### Phase 1 (现在): 最小化方案 ✅

```yaml
# 就用 kubeconfig + insecureSkipVerify
k8s:
  provider: "gcloud"
  kubeconfigPath: "~/.kube/config"
  insecureSkipVerify: true  # 测试环境 OK
```

优点:
- ✅ 无额外依赖
- ✅ 快速部署
- ✅ 适合开发/测试

缺点:
- ❌ 不安全 (TLS 验证关闭)
- ❌ 硬编码 IP 地址

### Phase 2: GCloud SDK 集成

```yaml
k8s:
  provider: "gcloud"
  gcloud:
    project: "my-project"
    zone: "asia-east1-b"
    cluster: "my-cluster"
```

优点:
- ✅ 完全自动化
- ✅ 安全 (TLS 验证开启)
- ✅ 无需手动修改 kubeconfig

缺点:
- ❌ 需要添加 `cloud.google.com/go` 依赖
- ❌ 更复杂的配置

### Phase 3: SSH 隧道支持

```yaml
k8s:
  provider: "gcloud"
  gcloud:
    instance: "my-vm"
    zone: "asia-east1-b"
    sshTunnel: true
```

优点:
- ✅ 支持自建 K8s on GCE
- ✅ 安全隧道连接

缺点:
- ❌ 后台隧道进程管理复杂

---

## 📚 参考资源

- [GCloud SDK 文档](https://cloud.google.com/docs/gcloud)
- [GKE 认证](https://cloud.google.com/kubernetes-engine/docs/how-to/api-server-authentication)
- [kubeadm 自签名证书](https://kubernetes.io/docs/tasks/administer-cluster/certificates/)
- [k8s.io/client-go 认证插件](https://pkg.go.dev/k8s.io/client-go@latest/plugin/pkg/client/auth)

---

## 🆘 故障排查

### 问题 1: TLS 证书验证失败

```
tls: failed to verify certificate: x509: certificate is valid for 10.140.0.2, not 35.236.172.169
```

**解决方案:**
1. 禁用 TLS 验证 (临时): `insecureSkipVerify: true`
2. 或重新生成证书包含外部 IP
3. 或使用 SSH 隧道 (安全方案)

### 问题 2: 无法连接 API Server

```
Unable to connect to the server: dial tcp 35.236.172.169:6443: connection refused
```

**检查清单:**
- [ ] 集群是否在运行? `gcloud compute instances list`
- [ ] API Server 端口是否开放? `gcloud compute firewall-rules list`
- [ ] kubeconfig 中的地址是否正确?
- [ ] 网络连通性? `telnet 35.236.172.169 6443`

### 问题 3: gcloud CLI 认证失败

```
ERROR: (gcloud.container.clusters.list) ResponseError: code=403
```

**解决方案:**
```bash
gcloud auth login
gcloud config set project YOUR_PROJECT_ID
```

---

**总结**: 当前的 kubeconfig + insecureSkipVerify 方案已足够 MVP 阶段。后续可根据需求逐步引入 GCloud SDK 和 SSH 隧道支持。
