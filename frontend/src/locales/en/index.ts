// English messages, one file per domain. Each domain file carries its own
// top-level groups; flatten here so keys resolve as chat.xxx, not chat.chat.xxx.
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
