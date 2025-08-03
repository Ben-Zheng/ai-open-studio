import requests
import os
import traceback
from notebook.base.handlers import IPythonHandler
from .config import aiservice_server,headers,workspace_id,instance_id

class durationHandler(IPythonHandler):
    # 获取已用时长
    def get(self):
        try:
            cookies = {key:str(self.cookies[key]) for key in self.cookies}
            url = f'{aiservice_server}/api/v1/lab/workspaces/{workspace_id}/instances/{instance_id}'
            response = requests.request("GET", url, headers=headers, cookies=cookies, verify=True)
            self.set_status(response.status_code)
            self.finish(response.json())
        except Exception as e:
            self.add_header("x-url-content",url)
            self.add_header("x-cookies-content",str(cookies))
            self.set_status(400)
            self.write(traceback.format_exc())
    def write_error(self, status_code, **kwargs):     # send_error  在其底层调用的是 write_error
        cookies = {key:str(self.cookies[key]) for key in self.cookies}
        url = f'{aiservice_server}/api/v1/lab/workspaces/{workspace_id}/instances/{instance_id}'
        self.add_header("x-url-content",url)
        self.add_header("x-cookies-content",str(cookies))
        self.set_status(status_code)
        self.write(traceback.format_exc())

