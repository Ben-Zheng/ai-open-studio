# workspace controller

workspace controller 是通过 kubebuilder 来生成的

## kubebuilder 简介
[kubebuilder](https://book.kubebuilder.io/quick-start.html) 是一个基于 CRD 来构建 Kubernetes API 的框架，可以使用 CRD 来构建 API、Controller 和 Admission Webhook。
具体构建步骤如下：
 1. 安装 kubebuilder
 2. 创建项目
    - 执行：kubebuilder init --domain my.domain --repo my.domain/guestbook --plugins=go/v4-alpha
 3. 构建 API
    - 执行：kubebuilder create api --group webapp --version v1 --kind Guestbook
 4. 修改 API 定义和控制器的相关逻辑
    - 修改 api/v1/guestbook_types.go 文件，可修改 crd 的相关结构定义
    - 修改 controllers/guestbook_controller.go 文件，可以修改控制器的协同控制逻辑
 5. 更新 API
    - 执行：make manifests，更新相关资源的配置文件到 config/crd/bases 目录下
    - 执行：make generate，更新 API 方法
 6. 构建控制器的镜像
    - 执行：make docker-build docker-push IMG=XXXXX
 7. 部署控制器
    - 执行：make install

***
# 相关工具的安装
主要包括 controller-gen kustomize envtest 工具的安装
## go version < go1.17
- make controller-gen kustomize envtest
## go version ≥ go1.17
- GOBIN=$(pwd)/bin GO111MODULE=on go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.7.0
- GOBIN=$(pwd)/bin GO111MODULE=on go install sigs.k8s.io/kustomize/kustomize/v3
- GOBIN=$(pwd)/bin GO111MODULE=on go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

***
# controller image 生成
## 方式一
- 修改 build.sh 中 image 名
- 执行 sh build.sh
## 方式二
- make docker-build docker-push IMG=docker-registry.i.brainpp.cn/brain/ais-workspace-controller:dev

***
# controller 修改
- 修改 controller 的相关功能或者定义后，可以执行 make generate 更新相关数据结构
- make manifests 可以更新相关资源的 yaml 定义

***
# controller 部署
- 相关资源定义在 config/crd 目录下
- 执行 make install 可完成部署