import requests
import os
import psutil
import multiprocessing

env = os.environ
info = psutil.virtual_memory()

headers = {
        'X-Token': env['AISERVICE_WORKSPACE_TOKEN'],
}

# 需要指定 3 个环境变量 MEGEDU_SERVER
def script_post_save(model, os_path, contents_manager, **kwargs):
    print("start")

c.ResourceUseDisplay.mem_limit = info.total
c.ResourceUseDisplay.track_cpu_percent = True
c.ResourceUseDisplay.cpu_limit = multiprocessing.cpu_count()

c.NotebookApp.allow_root = True
c.NotebookApp.base_url = env['AISERVICE_WORKSPACE_BASE_URL']
c.NotebookApp.disable_check_xsrf = True
c.NotebookApp.open_browser = False
c.NotebookApp.port_retries = 0
c.NotebookApp.terminado_settings = { "shell_command": ["/bin/bash"] }
c.NotebookApp.token = env['AISERVICE_WORKSPACE_TOKEN']
c.ConnectionFileMixin.ip = '0.0.0.0'
c.NotebookApp.port = 80

c.FileContentsManager.post_save_hook = script_post_save
c.FileCheckpoints.checkpoint_dir = '/home/aiservice/jupyter_checkpoint'
