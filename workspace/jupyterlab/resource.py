import argparse
import traceback
import pynvml
import psutil
import flask
import json
from flask import request

from loguru import logger


def get_gpu_info():
    results = []
    pynvml.nvmlInit()

    deviceCount = pynvml.nvmlDeviceGetCount()
    for i in range(deviceCount):
        result = {}

        # print("driverVersion: ", pynvml.nvmlSystemGetDriverVersion()) 
        result["driverVersion"] = str(pynvml.nvmlSystemGetDriverVersion(), 'utf-8')
        
        handle = pynvml.nvmlDeviceGetHandleByIndex(i)
        
        # print("uuid: ", pynvml.nvmlDeviceGetUUID(handle))
        result["uuid"] = str(pynvml.nvmlDeviceGetUUID(handle), 'utf-8')

        # print("types:", pynvml.nvmlDeviceGetName(handle))
        result["types"] = str(pynvml.nvmlDeviceGetName(handle), 'utf-8')

        # print("id:", pynvml.nvmlDeviceGetMinorNumber(handle))
        result["id"] = pynvml.nvmlDeviceGetMinorNumber(handle)

        # print("fanSpeed:", pynvml.nvmlDeviceGetFanSpeed(handle))
        result["fanSpeed"] = pynvml.nvmlDeviceGetFanSpeed(handle)
        
        # print("computeRunningProcesses:", len(pynvml.nvmlDeviceGetComputeRunningProcesses(handle)))
        result["computeRunningProcesses"] = len(pynvml.nvmlDeviceGetComputeRunningProcesses(handle))

        # print("graphicsRunningProcesses:", len(pynvml.nvmlDeviceGetGraphicsRunningProcesses(handle)))
        result["graphicsRunningProcesses"] = len(pynvml.nvmlDeviceGetGraphicsRunningProcesses(handle))

        # print("maxClock:", pynvml.nvmlDeviceGetMaxClockInfo(handle, 2))
        result["maxClock"] = pynvml.nvmlDeviceGetMaxClockInfo(handle, 2)
        
        # print("maxPcieLinkWidth:", pynvml.nvmlDeviceGetMaxPcieLinkWidth(handle))
        result["maxPcieLinkWidth"] = pynvml.nvmlDeviceGetMaxPcieLinkWidth(handle)
        
        # print("pcieThroughput,pcieRXThroughput:", pynvml.nvmlDeviceGetPcieThroughput(handle, 1))
        result["pcieThroughput"] = pynvml.nvmlDeviceGetPcieThroughput(handle, 1)
        result["pcieRXThroughput"] = pynvml.nvmlDeviceGetPcieThroughput(handle, 1)
        
        # print("pcieTXThroughput:", pynvml.nvmlDeviceGetPcieThroughput(handle, 0))
        result["pcieTXThroughput"] = pynvml.nvmlDeviceGetPcieThroughput(handle, 0)
        
        # print("performanceState:", pynvml.nvmlDeviceGetPerformanceState(handle))
        result["performanceState"] = pynvml.nvmlDeviceGetPerformanceState(handle)
        
        # print("powerManagementDefLimit:", pynvml.nvmlDeviceGetPowerManagementDefaultLimit(handle))
        result["powerManagementDefLimit"] = 1.0 * pynvml.nvmlDeviceGetPowerManagementDefaultLimit(handle)/1000
        
        # print("powerManagementLimit:", pynvml.nvmlDeviceGetPowerManagementLimit(handle))
        result["powerManagementLimit"] = 1.0 * pynvml.nvmlDeviceGetPowerManagementLimit(handle)/1000
        
        # print("powerUsage:", pynvml.nvmlDeviceGetPowerUsage(handle))
        result["powerUsage"] = 1.0 * pynvml.nvmlDeviceGetPowerUsage(handle)/1000
        
        # print("powerState:", pynvml.nvmlDeviceGetPowerState(handle))
        result["powerState"] = pynvml.nvmlDeviceGetPowerState(handle)
        
        # print("temp", pynvml.nvmlDeviceGetTemperature(handle,0))
        result["temp"] = pynvml.nvmlDeviceGetTemperature(handle,0)

        # print("temperatureThreshold", pynvml.nvmlDeviceGetTemperatureThreshold(handle, 1))
        result["temperatureThreshold"] = pynvml.nvmlDeviceGetTemperatureThreshold(handle, 1)
        
        util = pynvml.nvmlDeviceGetUtilizationRates(handle)
        # print("utilization", util.gpu)
        result["utilization"] = util.gpu
        # print("memUtilization", util.memory)
        result["memUtilization"] = util.memory
        
        info = pynvml.nvmlDeviceGetMemoryInfo(handle)
        # print("totalMem",info.total)
        result["totalMem"] = info.total
        # print("freeMem",info.free)
        result["freeMem"] = info.free
        # print("usedMem",info.used)
        result["usedMem"] = info.used

        results.append(result)
    pynvml.nvmlShutdown()
    return results


def get_resource():
    try:
        cpu_util = round(psutil.cpu_percent(interval=2))
        mem_util = round(psutil.virtual_memory()[2])
        mem_total = psutil.virtual_memory()[0]
        mem_used = psutil.virtual_memory()[0]-psutil.virtual_memory()[1]
    except Exception as e:
        traceback.format_exc()
        return

    result = {"cpu": {"util":cpu_util}, "mem": {"util": mem_util, "used": mem_used, "total": mem_total}}
    
    try:
        result["gpuInfo"] = get_gpu_info()
    except Exception as e:
        logger.info(traceback.format_exc())
        
    logger.info(result)
    return result


parser = argparse.ArgumentParser(description='Test for argparse')
parser.add_argument('--port', '-p', help='端口号，默认为 9874', default=9874)
args = parser.parse_args()

# 创建一个服务，把当前这个python文件当做一个服务
server = flask.Flask(__name__)


@server.route('/metrics', methods=['get'])
def metrics():
    resu = get_resource()
    return json.dumps(resu, ensure_ascii=False)


if __name__ == '__main__':
    try:
        server.run(debug=True, port=args.port, host='0.0.0.0')
    except Exception as e:
        logger.error(e)
