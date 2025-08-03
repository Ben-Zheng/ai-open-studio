**`kubevirt-gpu-device-plugin`**

用于给 `kubevirt-gpu-device-plugin` 打 patch。添加 COMMON_DEVICE_NAME 来支持统一指定多种类型 GPU 的 deviceName，注意指定值需要全部大写。

项目来源: [kubevirt-gpu-device-plugin](https://github.com/NVIDIA/kubevirt-gpu-device-plugin)，tag 为 v1.1.1、v1.2.2。kubelet 1.25 以上可使用 v1.1.1；1.25 及更新版本使用 v1.2.2。

打补丁顺序
- 0000-common-gpu-device-name-env.patch