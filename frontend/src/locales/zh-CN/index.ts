// 中文文案（默认），按域分文件。各域文件自带顶层组名，这里摊平成消息树；
// 再包一层会得到 chat.chat.xxx 双前缀，运行时一个 key 都解析不到。
import common from "./common"
import overview from "./overview"
import chat from "./chat"
import workspace from "./workspace"
import manage from "./manage"
import settings from "./settings"
import web from "./web"

export default {
  ...common,
  ...overview,
  ...chat,
  ...workspace,
  ...manage,
  ...settings,
  ...web,
}
