import QtQuick
import QtQuick.Controls
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui
import "Model.js" as Model

Panel {
  id: root
  moduleName: "evo.steam"
  ipcTarget: "evo.steam"
  manageIpc: false

  property var anchorItem: null
  property var hostWidget: null
  readonly property var barIdentity: hostWidget || root

  readonly property color foreground: Color.foreground
  readonly property color urgent: bar ? bar.urgent : Color.urgent
  readonly property color accent: Color.accent
  readonly property color dim: Qt.darker(foreground, 1.4)
  readonly property color surface: Color.popups.background
  readonly property string fontFamily: bar ? bar.fontFamily : Style.font.family

  readonly property int tileArtWidth: 96
  readonly property int tileArtHeight: 88
  readonly property int tileArtSourceSize: 192
  readonly property int tileHeight: tileArtHeight + 20
  readonly property int maxPlayedGames: 3
  readonly property int pollMs: 2000

  property bool loading: true
  property var data: Model.emptyData()

  readonly property var displayedGames: {
    var games = data && Array.isArray(data.playedGames) ? data.playedGames : []
    return games.slice(0, maxPlayedGames)
  }
  readonly property string statusScript: Qt.resolvedUrl("bin/steam-status").toString().replace("file://", "")
  readonly property bool iconActive: Model.iconActive(data)
  readonly property bool iconError: !loading && !!(data && data.error)
  readonly property bool iconBusy: iconActive
  readonly property bool iconMuted: !!(data && data.ok && !data.running)
  readonly property string barTooltip: Model.barTooltip(data)
  readonly property string statusPillText: Model.statusPillText(data, loading)
  readonly property color statusPillColor: Model.statusPillColor(data, loading, accent, urgent, foreground)

  function applyPayload(raw) {
    loading = false
    data = Model.parsePayload(raw)
  }

  function refresh() {
    if (!statusScript || statusProc.running) return
    if (!data.ok) loading = true
    statusProc.command = [statusScript, "popup"]
    statusProc.running = true
  }

  function launchGame(appid) {
    var id = String(appid || "").trim()
    if (!id || actionProc.running) return
    actionProc.command = [statusScript, "launch", id]
    actionProc.running = true
  }

  function openSteam() {
    if (actionProc.running) return
    actionProc.command = [statusScript, "open"]
    actionProc.running = true
  }

  function openFromHotkey() {
    root.controller.show()
    root.refresh()
  }

  function toggle() {
    if (root.opened) root.close()
    else root.openFromHotkey()
  }

  Component.onCompleted: refresh()

  onOpenedChanged: {
    if (opened) {
      refresh()
      pollTimer.start()
      Qt.callLater(function() { keyCatcher.forceActiveFocus() })
    } else {
      pollTimer.stop()
    }
  }

  Process {
    id: statusProc
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        var raw = String(text || "").trim()
        if (!raw) {
          root.loading = false
          return
        }
        root.applyPayload(raw)
      }
    }
    stderr: StdioCollector { waitForEnd: true }
    onExited: root.loading = false
  }

  Process {
    id: actionProc
  }

  Timer {
    id: pollTimer
    interval: root.pollMs
    repeat: true
    onTriggered: root.refresh()
  }



  IpcHandler {
    target: root.ipcTarget

    function open(): void { root.openFromHotkey() }
    function close(): void { root.close() }
    function show(): void { root.openFromHotkey() }
    function hide(): void { root.close() }
    function toggle(): void { root.toggle() }
    function refresh(): string { root.refresh(); return "ok" }
  }

  KeyboardPanel {
    id: panel
    anchorItem: root.anchorItem
    owner: root.barIdentity
    bar: root.bar
    open: root.opened
    focusTarget: keyCatcher
    contentWidth: panel.fittedContentWidth(Style.space(380))
    contentHeight: panel.fittedContentHeight(column.implicitHeight, Style.space(520))

    PanelKeyCatcher {
      id: keyCatcher
      anchors.fill: parent
      onCloseRequested: root.close()
      onTabRequested: function(direction) {
        if (root.bar && typeof root.bar.switchPanelFrom === "function")
          root.bar.switchPanelFrom(root.barIdentity, direction)
      }

      Flickable {
        id: panelFlick
        anchors.fill: parent
        contentWidth: width
        contentHeight: column.implicitHeight
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        flickableDirection: Flickable.VerticalFlick
        interactive: contentHeight > height
        ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

        Column {
          id: column
          width: panelFlick.width
          spacing: Style.space(12)

          PanelHero {
            width: parent.width
            title: "Steam"
            meta: root.data.running ? "Library" : "Not running"
            foreground: root.foreground
            fontFamily: root.fontFamily
            iconOpacity: root.data.running ? 1 : 0.55

            iconComponent: Component {
              Text {
                text: "󰓓"
                color: root.accent
                font.family: root.fontFamily
                font.pixelSize: Style.font.display
                opacity: 0.92
              }
            }
          }

          Row {
            width: parent.width
            spacing: Style.space(8)

            StatTile {
              width: (parent.width - parent.spacing * 2) / 3
              value: root.loading ? "…" : (root.data.installedCount <= 0 ? "—" : String(root.data.installedCount))
              label: "installed"
            }

            StatTile {
              width: (parent.width - parent.spacing * 2) / 3
              value: root.loading ? "…" : (root.data.ownedCount <= 0 ? "—" : String(root.data.ownedCount))
              label: "games"
            }

            StatTile {
              width: (parent.width - parent.spacing * 2) / 3
              value: Model.formatTotalPlayed(root.data.totalPlaytimeMin, root.loading)
              label: "total time"
            }
          }

          StatusPill {
            width: parent.width
            visible: root.statusPillText !== ""
            text: root.statusPillText
            textColor: root.statusPillColor
            foreground: root.foreground
            fontFamily: root.fontFamily
          }

          PanelSeparator {
            visible: root.displayedGames.length > 0
            foreground: root.foreground
          }

          PanelSectionHeader {
            visible: root.displayedGames.length > 0
            width: parent.width
            text: "RECENT"
            foreground: root.foreground
            fontFamily: root.fontFamily
          }

          Repeater {
            model: root.displayedGames

            GameRow {
              required property var modelData
              required property int index
              width: column.width
              game: modelData
              showDivider: index < root.displayedGames.length - 1
            }
          }

          Text {
            width: parent.width
            visible: !root.loading && !!(root.data && root.data.error)
            text: String((root.data && root.data.error) || "")
            color: root.urgent
            font.family: root.fontFamily
            font.pixelSize: Style.font.body
            wrapMode: Text.WordWrap
          }

          Text {
            width: parent.width
            visible: !root.loading && root.displayedGames.length === 0 && !(root.data && root.data.error)
            text: (root.data && root.data.running) ? "No recent play history" : "Steam not running"
            color: root.dim
            font.family: root.fontFamily
            font.pixelSize: Style.font.bodySmall
            horizontalAlignment: Text.AlignHCenter
          }
        }
      }
    }
  }

  component StatusPill: Rectangle {
    property string text: ""
    property color textColor: foreground
    property color foreground: Color.foreground
    property string fontFamily: Style.font.family

    implicitWidth: pillText.implicitWidth + Style.spacing.lg * 2
    implicitHeight: pillText.implicitHeight + Style.spacing.sm * 2
    radius: implicitHeight / 2
    color: Qt.rgba(textColor.r, textColor.g, textColor.b, 0.14)

    Text {
      id: pillText
      anchors.centerIn: parent
      text: parent.text
      color: parent.textColor
      font.family: parent.fontFamily
      font.pixelSize: Style.font.caption
      font.bold: true
    }
  }

  component GameRow: Item {
    id: gameRow
    property var game: null
    property bool showDivider: false

    implicitHeight: root.tileHeight + (showDivider ? 1 : 0)

    Rectangle {
      anchors.bottom: parent.bottom
      width: parent.width
      height: 1
      visible: gameRow.showDivider
      color: Qt.rgba(root.foreground.r, root.foreground.g, root.foreground.b, 0.12)
    }

    Item {
      anchors.top: parent.top
      width: parent.width
      height: root.tileHeight

      scale: tileMouse.pressed ? 0.97 : 1
      opacity: tileMouse.pressed ? 0.88 : 1

      Behavior on scale { NumberAnimation { duration: 90; easing.type: Easing.OutCubic } }
      Behavior on opacity { NumberAnimation { duration: 90 } }

      Rectangle {
        anchors.fill: parent
        radius: Style.cornerRadius
        color: tileMouse.pressed
          ? Qt.rgba(root.accent.r, root.accent.g, root.accent.b, 0.12)
          : (tileMouse.containsMouse
            ? Qt.rgba(root.foreground.r, root.foreground.g, root.foreground.b, 0.07)
            : "transparent")
      }

      Row {
        anchors.fill: parent
        anchors.margins: Style.space(10)
        spacing: Style.space(12)

        Item {
          width: root.tileArtWidth
          height: root.tileArtHeight
          clip: true

          Text {
            anchors.centerIn: parent
            visible: gameIcon.source === "" || gameIcon.status === Image.Error
            text: "󰓓"
            color: root.foreground
            opacity: 0.35
            font.family: root.fontFamily
            font.pixelSize: Style.font.title
          }

          Image {
            id: gameIcon
            anchors.fill: parent
            visible: source !== "" && status !== Image.Error
            source: Model.gameArtUrl(gameRow.game ? gameRow.game.icon_path : "")
            fillMode: Image.PreserveAspectCrop
            asynchronous: true
            cache: true
            smooth: true
            mipmap: true
            sourceSize: Qt.size(
              root.tileArtSourceSize,
              Math.round(root.tileArtSourceSize * root.tileArtHeight / root.tileArtWidth))
          }
        }

        Column {
          width: parent.width - root.tileArtWidth - Style.space(12) - playHint.implicitWidth - Style.space(8)
          spacing: Style.spacing.labelGap
          anchors.verticalCenter: parent.verticalCenter

          Text {
            width: parent.width
            text: String(gameRow.game ? gameRow.game.name : "Game")
            color: tileMouse.containsMouse ? root.accent : root.foreground
            font.family: root.fontFamily
            font.pixelSize: Style.font.title
            font.bold: true
            elide: Text.ElideRight
            maximumLineCount: 2
            wrapMode: Text.Wrap
          }

          Row {
            spacing: Style.spacing.sm

            StatusPill {
              visible: Model.formatLastPlayed(gameRow.game ? gameRow.game.last_played : 0) !== "—"
              text: Model.formatLastPlayed(gameRow.game ? gameRow.game.last_played : 0)
              textColor: root.foreground
              foreground: root.foreground
              fontFamily: root.fontFamily
            }

            StatusPill {
              visible: Model.formatPlaytime(gameRow.game ? gameRow.game.playtime_min : 0) !== ""
              text: Model.formatPlaytime(gameRow.game ? gameRow.game.playtime_min : 0)
              textColor: root.foreground
              foreground: root.foreground
              fontFamily: root.fontFamily
            }
          }
        }

        Text {
          id: playHint
          visible: tileMouse.containsMouse && !tileMouse.pressed
          anchors.verticalCenter: parent.verticalCenter
          text: "󰐊"
          color: root.accent
          opacity: 0.85
          font.family: root.fontFamily
          font.pixelSize: Style.font.title
        }
      }

      MouseArea {
        id: tileMouse
        anchors.fill: parent
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onClicked: root.launchGame(gameRow.game ? gameRow.game.appid : "")
      }
    }
  }

  component StatTile: BorderSurface {
    id: tile
    property string value: ""
    property string label: ""
    property color valueColor: root.accent

    implicitHeight: tileColumn.implicitHeight + Style.spacing.lg * 2
    color: Color.popups.background
    borderSpec: Border.surfaceSpec("popups", "border", Color.popups.border, 1)
    radius: Style.cornerRadius

    Column {
      id: tileColumn
      anchors.centerIn: parent
      width: parent.width - Style.spacing.lg * 2
      spacing: Style.spacing.labelGap

      Text {
        width: parent.width
        text: tile.value
        color: tile.valueColor
        font.family: root.fontFamily
        font.pixelSize: Style.font.title
        font.bold: true
        horizontalAlignment: Text.AlignHCenter
        elide: Text.ElideRight
      }

      Text {
        width: parent.width
        text: tile.label
        color: root.dim
        font.family: root.fontFamily
        font.pixelSize: Style.font.caption
        horizontalAlignment: Text.AlignHCenter
        elide: Text.ElideRight
      }
    }
  }
}
