import {
  ILayoutRestorer,
  JupyterFrontEnd,
  JupyterFrontEndPlugin,
} from '@jupyterlab/application';
import {ServerConnection} from "@jupyterlab/services";
import { ISettingRegistry } from '@jupyterlab/settingregistry';
import {IDocumentManager} from '@jupyterlab/docmanager';
import {IFileBrowserFactory} from '@jupyterlab/filebrowser';
import {IStatusBar} from '@jupyterlab/statusbar';
import { IMainMenu } from '@jupyterlab/mainmenu';
import { Menu, Widget } from '@lumino/widgets';
import {
  MainAreaWidget,
  WidgetTracker
} from '@jupyterlab/apputils';

import {Heartbeat} from "./heartbeat";
import Settings from "./settings";
import "../style/index.css";
import { Resource } from './resource';

const PLUGIN_ID = 'megzilla:brain';

class RefWidget extends Widget {
  readonly htmlIF: HTMLIFrameElement;
  constructor() {
    super();
    this.addClass('megengine-Widget');
    this.htmlIF = document.createElement('iframe');
    this.htmlIF.src = 'https://megengine.org.cn/doc/stable/zh/reference/index.html';
    this.htmlIF.onload = this.onload;
    this.htmlIF.width =
      (1 * document.documentElement.clientWidth).toString() + 'px';
    this.htmlIF.height =
      document.documentElement.clientHeight.toString() + 'px';

    this.node.appendChild(this.htmlIF);
  }

  onload() {
    this.htmlIF.width =
      document.documentElement.clientWidth.toString() + 'px';
    this.htmlIF.height =
      document.documentElement.clientHeight.toString() + 'px';
  }
}

const extension: JupyterFrontEndPlugin<void> = {
  id: PLUGIN_ID,
  autoStart: true,
  requires: [
    IDocumentManager,
    IFileBrowserFactory,
    ILayoutRestorer,
    ISettingRegistry,
    IStatusBar,
    IMainMenu,
  ],
  activate:activate
};

function activate(app: JupyterFrontEnd, manager: IDocumentManager, factory: IFileBrowserFactory, restorer: ILayoutRestorer,
  settingRegistry: ISettingRegistry, status: IStatusBar, mainMenu: IMainMenu) {
  app.started.then(()=>{
    manager.services.contents.get("work.ipynb", { content: true }).then(() => {
      manager.open("work.ipynb");
    });
    manager.autosave = true;
    manager.autosaveInterval = 10;
    console.log(manager.autosaveInterval);
  });

  // 与剩余时长有关的加载
  const hb = new Heartbeat();
  status.registerStatusItem('compute-power-duration', {
    item: hb,
    align: 'left',
    rank: 3,
    isActive: () => {
      return true
    }
  });

  // 与资源利用情况有关的加载
  const jpre = new Resource();
  status.registerStatusItem('resource', {
    item: jpre,
    align: 'left',
    rank: 4,
    isActive: () => {
      return true
    }
  });

  // megengine reference
  let widgetR: MainAreaWidget<RefWidget>;
  const commandR = 'megengine:open';
  app.commands.addCommand(commandR, {
    label: 'MegEngine Reference',
    execute: () => {
      if (!widgetR || widgetR.isDisposed) {
        const content = new RefWidget();
        widgetR = new MainAreaWidget({ content });
        widgetR.id = 'megengine-reference';
        widgetR.title.label = 'aiservice-megengine-reference';
        widgetR.title.closable = true;
      }
      if (!trackerR.has(widgetR)) {
        trackerR.add(widgetR);
      }
      if (!widgetR.isAttached) {
        app.shell.add(widgetR, 'main');
      }
      widgetR.content.update();
      app.shell.activateById(widgetR.id);
    }
  });
  const trackerR = new WidgetTracker<MainAreaWidget<RefWidget>>({
    namespace: 'megengine'
  });
  restorer.restore(trackerR, { command: commandR, name: () => 'megengine' });
  const helpMenu = new Menu({ commands: app.commands });
  helpMenu.title.label = 'megengine reference';
  mainMenu.helpMenu.addGroup([{ type: 'submenu', submenu: helpMenu }], 0);
  helpMenu.addItem({ command: commandR });

  // 与项目无操作退出时间相关
  let config = new Settings();

  function loadSetting() {
    let settings = ServerConnection.makeSettings();
    ServerConnection.makeRequest(settings.baseUrl + 'settings', {method: 'GET'}, settings).then((response) => {
      if (response.status !== 200){ return }
      response.json().then((data) => {
        config = data.data;
      });
    });
  }

  function updateSettings(num:number) {
    let settings = ServerConnection.makeSettings();
    config.exitTimeout = num;
    ServerConnection.makeRequest(settings.baseUrl + 'settings', {method: 'POST', body: JSON.stringify(config)}, settings)
  }


  Promise.all([app.restored, settingRegistry.load(PLUGIN_ID)])
    .then(([, setting]) => {
      loadSetting();

      app.commands.addCommand("m10", {
        label: '10分钟后',
        isToggled: () => config.exitTimeout == 600,
        execute: () => {
          setting.set('exitTimeout', 600).then(() => {
            updateSettings(600);
          });
        }
      });

      app.commands.addCommand("h1", {
        label: '1小时后',
        isToggled: () => config.exitTimeout == 3600,
        execute: () => {
          setting.set('exitTimeout', 3600).then(() => {
            updateSettings(3600);
          });
        }
      });

      app.commands.addCommand("h2", {
        label: '2小时后',
        isToggled: () => config.exitTimeout == 7200,
        execute: () => {
          setting.set('exitTimeout', 7200).then(() => {
            updateSettings(7200);
          });
        }
      });

      app.commands.addCommand("h10", {
        label: '10小时后',
        isToggled: () => config.exitTimeout == 36000,
        execute: () => {
          setting.set('exitTimeout', 36000).then(() => {
            updateSettings(36000);
          });
        }
      });

      const settingsMenu = new Menu({commands:app.commands});
      settingsMenu.title.label = '页面无操作后环境终止时间';
      mainMenu.settingsMenu.addGroup([
        { type: 'submenu', submenu: settingsMenu }
      ], 0);

      settingsMenu.addItem({command: "m10"});
      settingsMenu.addItem({command: "h1"});
      settingsMenu.addItem({command: "h2"});
      settingsMenu.addItem({command: "h10"});
    })
    .catch(reason => {
      console.error(
        `Something went wrong when reading the settings.\n${reason}`
      );
    });

  return;
}

export default extension;
