import traceback
import pynvml
import psutil
from notebook.base.handlers import IPythonHandler

class resourceHandler(IPythonHandler):
    # 获取剩余可用时长
    def get(self):
        try:
            cpu_util = round(psutil.cpu_percent(interval=2))
            mem_util = round(psutil.virtual_memory()[2])
            mem_total = round(psutil.virtual_memory()[0]/1024/1024/1024,1)
            mem_used = round((psutil.virtual_memory()[0]-psutil.virtual_memory()[1])/1024/1024/1024,1)
            gpu_util = 0
            gpu_mem_util = 0
            gpu_mem_total = 0
            gpu_mem_used = 0
        except Exception as e:
            self.set_status(400)
            self.write(traceback.format_exc())
            return

        try:
            pynvml.nvmlInit()
            deviceCount = pynvml.nvmlDeviceGetCount()
            for i in range(deviceCount):
                handle = pynvml.nvmlDeviceGetHandleByIndex(i)
                meminfo = pynvml.nvmlDeviceGetMemoryInfo(handle)
                gpu_mem_used += meminfo.used
                gpu_mem_total += meminfo.total
                gpu_util += pynvml.nvmlDeviceGetUtilizationRates(handle).gpu
            # print(gpu_mem_used,gpu_mem_total,gpu_mem_used/gpu_mem_total,round(gpu_mem_used/gpu_mem_total))
            gpu_util = round(gpu_util/deviceCount)
            gpu_mem_util = round(100*gpu_mem_used/gpu_mem_total)
            gpu_mem_used = round(gpu_mem_used/1024/1024/1024,1)
            gpu_mem_total = round(gpu_mem_total/1024/1024/1024,1)
        except Exception as e:
            # print(traceback.format_exc())
            result = "CPU: {}%; 内存: {}% ({} GB/{} GB); GPU: 0%; 显存: 0% (0.0 GB/0.0 GB)".format(cpu_util,mem_util,mem_used,mem_total)
        else:
            result = "CPU: {}%; 内存: {}% ({} GB/{} GB); GPU: {}%; 显存: {}% ({} GB/{} GB)".format(cpu_util,mem_util,mem_used,mem_total,gpu_util,gpu_mem_util,gpu_mem_used,gpu_mem_total)
        # print(result)

        self.set_status(200)
        self.finish(result)

    def write_error(self, status_code, **kwargs):     # send_error  在其底层调用的是 write_error
        self.set_status(status_code)
        self.write(traceback.format_exc())
