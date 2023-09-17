# 安装operator-sdk

    brew install operator-sdk
    # 添加命令补全
    source <(operator-sdk completion zsh)

# 创建项目
## 创建项目及初始化

    #创建一个目录
    mkdir operator-example
    cd operator-example
    go mod init
    operator-sdk init  --domain tsdb.top
    go mod tidy

已经生成一些初始化目录和文件

    ├── Dockerfile
    ├── Makefile #用于构建和部署 controller
    ├── PROJECT #用于创建新组件的元数据
    ├── README.md
    ├── cmd
    │   └── main.go #程序的入口
    ├── config # 运行 operator 所需的所有配置文件
    │   ├── default
    │   │   ├── kustomization.yaml
    │   │   ├── manager_auth_proxy_patch.yaml
    │   │   └── manager_config_patch.yaml
    │   ├── manager
    │   │   ├── kustomization.yaml
    │   │   └── manager.yaml
    │   ├── prometheus # 监控相关
    │   │   ├── kustomization.yaml
    │   │   └── monitor.yaml
    │   └── rbac #包含运行 controller 所需最小权限的配置文件
    │       ├── auth_proxy_client_clusterrole.yaml
    │       ├── auth_proxy_role.yaml
    │       ├── auth_proxy_role_binding.yaml
    │       ├── auth_proxy_service.yaml
    │       ├── kustomization.yaml
    │       ├── leader_election_role.yaml
    │       ├── leader_election_role_binding.yaml
    │       ├── role_binding.yaml
    │       └── service_account.yaml
    ├── go.mod
    ├── go.sum
    └── hack # 此目录旨在存储基本的shell脚本或任何其他类型的hacky脚本，以自动化我们oeprator周围的任何类型的操作。例如，运行某些检查背后的脚本安装和设置必备工具的脚本等都将放置在这里。
    └── boilerplate.go.txt

## 创建API
运行下面的命令，创建一个新的 API（组/版本）为 "web/v1"，并在上面创建新的 Kind(CRD) Frontend

    operator-sdk create api --group web --version v1 --kind Frontend --resource  --controller 

就会生成`api/v1/frontend_types.go`,该文件中定义相关 API,而针对于这一类型 (CRD) 的业务逻辑生成在`controller/frontend_controller.go`文件中。`config/samples/web_v1_frontend.yaml`该文件是API样例yaml文件。


实现一个daemonset资源对象的operator，作用为在每个节点启动一个server提供api服务，并且定期的备份数据库到s3存储

## 添加字段
    type Backup struct {
        Schedule string `json:"schedule,omitempty"`
    }
    
    type FrontendSpec struct {
        // 项目配置文件
        Config string `json:"config,omitempty"`
        // +kubebuilder:validation:Required
        Image  string     `json:"image"`
        Backup Backup `json:"backup,omitempty"`
    }
    
    type FrontendStatus struct {
        // 最后一次备份数据库的时间
        LastScheduleTime metav1.Time `json:"lastScheduleTime,omitempty"`
    }

### 生成 crd
    # 生成crd描述文件，在前目录的config/crd/bases目录下
    make manifests
    
    # 生成DeepCopy代码
    make generate

## 部署crd到k8s中
    make install
    
    # 查看字段
    kubectl explain frontends.web.tsdb.top --recursive=1

## 添加测试数据
    apiVersion: web.tsdb.top/v1
    kind: Frontend
    metadata:
      labels:
        app.kubernetes.io/name: frontend
        app.kubernetes.io/instance: frontend-sample
      name: frontend-sample
    spec:
      backup:
        schedule: 30 2 * * * // 每天2点30备份数据库
      image: registry.tsdb.top:5000/api-example:latest
      config: |-
        db: mysql://root:xxxx@tcp(0.0.0.0:3306)/test?charset=utf8&loc=Local&parseTime=true

