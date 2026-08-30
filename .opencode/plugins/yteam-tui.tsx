import type { TuiPlugin, TuiPluginModule } from "@opencode-ai/plugin/tui"

const tui: TuiPlugin = async (api) => {
  api.slots.register({
    slots: {
      home_logo() {
        return (
          <box flexDirection="column">
            <text fg="white">YTEAM</text>
            <text fg="cyan">██╗   ██╗████████╗███████╗ █████╗ ███╗   ███╗</text>
            <text fg="cyan">╚██╗ ██╔╝╚══██╔══╝██╔════╝██╔══██╗████╗ ████║</text>
            <text fg="blue"> ╚████╔╝    ██║   █████╗  ███████║██╔████╔██║</text>
            <text fg="blue">  ╚██╔╝     ██║   ██╔══╝  ██╔══██║██║╚██╔╝██║</text>
            <text fg="magenta">   ██║      ██║   ███████╗██║  ██║██║ ╚═╝ ██║</text>
            <text fg="magenta">   ╚═╝      ╚═╝   ╚══════╝╚═╝  ╚═╝╚═╝     ╚═╝</text>
            <text fg="gray">security engineering · bug bounty · pentest · QA</text>
          </box>
        )
      },
      home_bottom() {
        return (
          <box width="100%" justifyContent="center" paddingTop={1} flexDirection="column">
            <box justifyContent="center">
              <text fg="cyan">YTEAM</text>
              <text fg="gray"> · secure operator console · loopback Hermes bridge</text>
            </box>
            <box justifyContent="center">
              <text fg="green">SAFE MODE</text>
              <text fg="gray"> · scope-first · read-only · low-rate · no auto-submit</text>
            </box>
            <box justifyContent="center">
              <text fg="yellow">BOTTERDOP</text>
              <text fg="gray"> detect → classify → slow_down/manual_review/stop</text>
              <text fg="gray"> · Camoufox: optional isolated observer</text>
            </box>
            <box justifyContent="center">
              <text fg="magenta">/bb &lt;authorized-target&gt;</text>
              <text fg="gray"> · other slash commands remain native OpenCode</text>
            </box>
          </box>
        )
      },
    },
  })
}

const plugin: TuiPluginModule & { id: string } = {
  id: "yteam-tui",
  tui,
}

export default plugin
