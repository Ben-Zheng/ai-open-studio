import {PanelLayout, Widget} from "@lumino/widgets";
import {ServerConnection} from "@jupyterlab/services";
export class Instance {
  id: number;
  workspaceID: number;
  usedDuration: number;
}

export class Heartbeat extends Widget {
  _text: HTMLSpanElement;
  instance: Instance;
  isShowDialogNoDuration = true;
  isShowDialogDurationInsufficient = true;

  constructor() {
    super();
    const layout = (this.layout = new PanelLayout());
    const wrapper = new Widget();
    wrapper.addClass("jp-statusbar");
    this._text = document.createElement('span');
    wrapper.node.appendChild(this._text);
    layout.addWidget(wrapper);
    this.refresh();
    setInterval(
      ()=> {
        this.refresh();
        },
      10000);
  }

  closeCurrentPage() {
    const ua = window.navigator.userAgent;
    if (ua.indexOf('MSIE') > 0) {
      if (ua.indexOf('MSIE 6.0') > 0) {
        window.opener = null;
        window.close();
      } else {
        window.open('', '_top');
        window.top.close();
      }
    } else {
      window.opener = null;
      window.open('', '_self', '');
      window.close();
    }
  }

  refresh() {
    let settings = ServerConnection.makeSettings();
    ServerConnection.makeRequest(settings.baseUrl + 'duration', {method: 'GET'}, settings).then((response) => {
      if (response.status != 200) {
        return
      }
      response.json().then((data) => {
        this.instance = data.data;
        this._text.innerText = `已使用时间: ${this.convert()}`;
      }).catch((e) => {
        console.log(e);
      });
    });
  }

  convert() {
    const val = this.instance.usedDuration;

    if (val < 36000) {
      this._text.style.color = '#0b0';
    } else if (val < 360000) {
      this._text.style.color = '#004';
    } else {
      this._text.style.color = '#e08';
    }
    if (val < 0) {
      return "00:00";
    }
    const minute = 60;
    const hour = 3600;
    const minC = Math.floor(val % hour / minute);
    const hourC = Math.floor(val / hour);
    // const secC = val % minute;
    let h : number | string;
    hourC < 10? h = ('0' + hourC).slice(-2): h = hourC;
    return h + ':' + ('00' + minC).slice(-2);
  }
}
