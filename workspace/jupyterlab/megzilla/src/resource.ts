import {PanelLayout, Widget} from "@lumino/widgets";
import {ServerConnection} from "@jupyterlab/services";

export class Resource extends Widget {
  _text: HTMLSpanElement;

  constructor() {
    super();
    const layout = (this.layout = new PanelLayout());
    const wrapper = new Widget();
    wrapper.addClass("jp-statusbar");
    this._text = document.createElement('span');
    wrapper.node.appendChild(this._text);
    layout.addWidget(wrapper);
    this.getresource();
    setInterval(
      ()=> {
        this.getresource();
        },
      10000);
  }

  getresource() {
    let settings = ServerConnection.makeSettings();
    ServerConnection.makeRequest(settings.baseUrl + 'resource', {method: 'GET'}, settings).then((response) => {
      if (response.status != 200) {
        return
      }
      response.text().then((data) => {
        this._text.style.color = '#00CCFF';
        this._text.innerText = `${data}`;
      }).catch((e) => {
        console.log(e);
      });
    });
  }
}
