import tornado
import requests
from notebook.base.handlers import IPythonHandler
from .config import aiservice_server,headers,workspace_id


class settingsHandler(IPythonHandler):
    def get(self):
        try:
            cookies = {key:str(self.cookies[key]) for key in self.cookies}
            url = f'{aiservice_server}/api/v1/lab/workspaces/{workspace_id}/settings'
            response = requests.request("GET", url, headers=headers, cookies=cookies, verify=True)
            self.set_status(response.status_code)
            self.finish(response.json())
        except Exception as e:
            self.add_header("x-url-content",url)
            self.add_header("x-cookies-content",str(cookies))
            self.set_status(400)
            self.write(traceback.format_exc())
    def post(self):
        try:
            cookies = {key:str(self.cookies[key]) for key in self.cookies}
            data = tornado.escape.json_decode(self.request.body)

            url = f'{aiservice_server}/api/v1/lab/workspaces/{workspace_id}/settings'
            response = requests.request("POST", url, headers=headers, json=data, cookies=cookies, verify=True)
            self.set_status(response.status_code)
            self.finish(response.json())
        except Exception as e:
            self.add_header("x-url-content",url)
            self.add_header("x-cookies-content",str(cookies))
            self.set_status(400)
            self.write(traceback.format_exc())
    def write_error(self, status_code, **kwargs):     # send_error  在其底层调用的是 write_error
        cookies = {key:str(self.cookies[key]) for key in self.cookies}
        url = f'{aiservice_server}/api/v1/lab/workspaces/{workspace_id}/settings'
        self.add_header("x-url-content",url)
        self.add_header("x-cookies-content",str(cookies))
        self.set_status(status_code)
        self.write(traceback.format_exc())
