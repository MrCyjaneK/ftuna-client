import QtQuick 2.6
import Sailfish.Silica 1.0
import "../ui.js" as Ui

// Install and update both land here. The list is the change. Yes starts it.
// Output is what PackageKit reports after that.
Page {
    id: page
    property var backend
    property string action: "Install"
    property var packages: []

    property bool decided: false
    property bool working: false
    property bool failed: false
    property bool checking: false
    property string previewError: ""
    property string changeText: ""
    property int step: 0
    property var queue: []
    property var plan: []
    property string outputText: ""

    function pushLine(text) {
        text = Ui.trim(text)
        if (text === "")
            return
        if (outputText === text || (outputText.length > text.length && outputText.lastIndexOf("\n" + text) === outputText.length - text.length - 1))
            return
        outputText = outputText === "" ? text : outputText + "\n" + text
        scrollTimer.restart()
    }

    function groups() {
        var order = []
        var map = {}
        for (var i = 0; i < packages.length; i++) {
            var pkg = packages[i]
            var specs = pkg.specs || []
            if (specs.length === 0)
                continue
            var url = pkg.repo_url || ""
            if (!map[url]) {
                map[url] = []
                order.push(url)
            }
            for (var s = 0; s < specs.length; s++)
                map[url].push(specs[s])
        }
        var out = []
        for (var g = 0; g < order.length; g++)
            out.push({ repo_url: order[g], specs: map[order[g]] })
        return out
    }

    function addChanges(rows) {
        var lines = []
        if (changeText !== "")
            lines.push(changeText)
        var list = rows || []
        for (var i = 0; i < list.length; i++) {
            var row = list[i]
            var line = Ui.trim((row.action || "") + "  " + (row.label || ""))
            if (line === "")
                continue
            if (row.summary)
                line += "  " + row.summary
            lines.push(line)
        }
        changeText = lines.join("\n")
    }

    function beginPreview() {
        queue = groups()
        plan = []
        step = 0
        changeText = ""
        previewError = ""
        if (queue.length === 0) {
            checking = false
            previewError = "Nothing selected"
            return
        }
        checking = true
        previewCurrent()
    }

    function previewCurrent() {
        var group = queue[step]
        backend.preview(group.repo_url, group.specs)
    }

    function sayNo() {
        pageStack.pop()
    }

    function sayYes() {
        if (decided || working || checking || previewError !== "" || plan.length === 0)
            return
        decided = true
        failed = false
        outputText = ""
        step = 0
        working = true
        pushLine(action)
        run()
    }

    function run() {
        var ids = []
        var src = plan[step] && plan[step].ids ? plan[step].ids : []
        for (var i = 0; i < src.length; i++)
            ids.push(src[i])
        backend.installPackages(ids)
    }

    function finish(result) {
        working = false
        if (result === "Installed" && action === "Update")
            pushLine("Updated")
        else
            pushLine(result)
        failed = result !== "Installed"
    }

    Timer {
        id: scrollTimer
        interval: 0
        onTriggered: view.positionViewAtEnd()
    }

    Connections {
        target: backend
        onActivityChanged: {
            if (page.working)
                page.pushLine(backend.activity)
        }
        onLogLine: {
            if (page.working)
                page.pushLine(line)
        }
        onChangesReady: {
            var ids = []
            var src = backend.resolvedIds || []
            for (var i = 0; i < src.length; i++)
                ids.push(src[i])
            var next = page.plan.slice(0)
            next.push({ ids: ids })
            page.plan = next
            page.addChanges(changes)
            page.step++
            if (page.step < page.queue.length)
                page.previewCurrent()
            else {
                page.checking = false
                if (page.changeText === "")
                    page.previewError = "No package changes"
            }
        }
        onChangesFailed: {
            page.checking = false
            page.previewError = error || "Could not check changes"
        }
        onInstallResultChanged: {
            if (!page.working)
                return
            var result = backend.installResult || ""
            if (result === "" || result.indexOf("Wait for") === 0)
                return
            if (result !== "Installed") {
                page.finish(result)
                return
            }
            page.step++
            if (page.step < page.queue.length)
                page.run()
            else
                page.finish(result)
        }
    }

    Component.onCompleted: beginPreview()

    SilicaListView {
        id: view
        anchors.fill: parent
        clip: true
        bottomMargin: dock.height
        model: page.packages
        currentIndex: -1

        header: PageHeader {
            title: page.action
        }

        delegate: ListItem {
            id: row
            contentHeight: Math.max(Theme.itemSizeLarge, body.height + Theme.paddingLarge)
            width: view.width
            readonly property color muted: highlighted ? Theme.secondaryHighlightColor : Theme.secondaryColor
            readonly property var lines: modelData.lines || []

            Image {
                id: icon
                anchors.left: parent.left
                anchors.leftMargin: Theme.horizontalPageMargin
                anchors.verticalCenter: parent.verticalCenter
                width: Theme.iconSizeMedium
                height: width
                fillMode: Image.PreserveAspectFit
                source: Ui.iconSource(modelData.icon_path, "image://theme/icon-m-other")
            }
            Column {
                id: body
                anchors.left: icon.right
                anchors.leftMargin: Theme.paddingMedium
                anchors.right: parent.right
                anchors.rightMargin: Theme.horizontalPageMargin
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.paddingSmall

                Label {
                    width: parent.width
                    truncationMode: TruncationMode.Fade
                    color: row.highlighted ? Theme.highlightColor : Theme.primaryColor
                    text: modelData.summary || modelData.id || ""
                }
                Item {
                    width: parent.width
                    height: Math.max(fromText.height, toText.height)
                    visible: row.lines.length === 0 && (fromText.text !== "" || toText.text !== "")
                    clip: true

                    Label {
                        id: fromText
                        anchors.verticalCenter: parent.verticalCenter
                        width: text === "" ? 0
                               : (toText.text === "" ? parent.width
                                  : Math.min(implicitWidth, Math.floor((parent.width - arrowText.implicitWidth - Theme.paddingMedium) / 2)))
                        font.pixelSize: Theme.fontSizeSmall
                        color: row.muted
                        truncationMode: TruncationMode.Fade
                        text: modelData.installed || ""
                    }
                    Label {
                        id: arrowText
                        anchors.left: fromText.right
                        anchors.leftMargin: fromText.text !== "" && toText.text !== "" ? Theme.paddingSmall : 0
                        anchors.verticalCenter: parent.verticalCenter
                        font.pixelSize: Theme.fontSizeSmall
                        color: row.muted
                        text: fromText.text !== "" && toText.text !== "" ? "→" : ""
                    }
                    Label {
                        id: toText
                        anchors.left: arrowText.right
                        anchors.leftMargin: text !== "" && arrowText.text !== "" ? Theme.paddingSmall : 0
                        anchors.right: parent.right
                        anchors.verticalCenter: parent.verticalCenter
                        font.pixelSize: Theme.fontSizeSmall
                        truncationMode: TruncationMode.Fade
                        color: row.highlighted ? Theme.highlightColor : Theme.primaryColor
                        text: modelData.version || ""
                    }
                }
                Repeater {
                    model: row.lines
                    Label {
                        width: body.width
                        truncationMode: TruncationMode.Fade
                        font.pixelSize: Theme.fontSizeSmall
                        color: row.muted
                        text: modelData
                    }
                }
                Label {
                    width: parent.width
                    truncationMode: TruncationMode.Fade
                    font.pixelSize: Theme.fontSizeSmall
                    color: Ui.repoColor(modelData.repo, row.muted)
                    text: modelData.repo || ""
                    visible: text !== ""
                }
            }
        }

        footer: Item {
            width: view.width
            height: detailColumn.childrenRect.height

            Column {
                id: detailColumn
                width: parent.width

                SectionHeader {
                    text: "Changes"
                    visible: page.checking || page.previewError !== "" || page.changeText !== ""
                }
                Label {
                    x: Theme.horizontalPageMargin
                    width: parent.width - 2 * x
                    visible: page.checking || page.previewError !== "" || page.changeText !== ""
                    wrapMode: Text.Wrap
                    font.pixelSize: Theme.fontSizeSmall
                    color: page.previewError !== "" ? "#e15b64" : Theme.secondaryColor
                    text: page.checking ? "Checking changes" : (page.previewError !== "" ? page.previewError : page.changeText)
                }
                SectionHeader {
                    text: "Output"
                    visible: page.outputText !== ""
                }
                Label {
                    x: Theme.horizontalPageMargin
                    width: parent.width - 2 * x
                    visible: page.outputText !== ""
                    wrapMode: Text.Wrap
                    font.pixelSize: Theme.fontSizeSmall
                    color: page.failed ? "#e15b64" : (page.working ? Theme.highlightColor : Theme.secondaryColor)
                    text: page.outputText
                }
                Item {
                    width: 1
                    height: Theme.paddingLarge
                }
            }
        }

        ViewPlaceholder {
            enabled: view.count === 0
            text: "Nothing selected"
        }

        VerticalScrollDecorator {}
    }

    Rectangle {
        id: dock
        z: 2
        anchors.bottom: parent.bottom
        width: parent.width
        height: !page.decided ? answers.height : (page.working ? workColumn.height : 0)
        visible: height > 0
        color: Theme.overlayBackgroundColor

        Column {
            id: workColumn
            width: parent.width
            visible: page.working

            Item {
                width: parent.width
                height: Theme.itemSizeSmall
                visible: backend.progress <= 0

                BusyIndicator {
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.horizontalPageMargin
                    anchors.verticalCenter: parent.verticalCenter
                    size: BusyIndicatorSize.Small
                    running: page.working && backend.progress <= 0
                }
                Label {
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.horizontalPageMargin + Theme.iconSizeSmall + Theme.paddingMedium
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.horizontalPageMargin
                    anchors.verticalCenter: parent.verticalCenter
                    truncationMode: TruncationMode.Fade
                    font.pixelSize: Theme.fontSizeSmall
                    color: Theme.highlightColor
                    text: backend.activity !== "" ? backend.activity : "Working"
                }
            }
            ProgressBar {
                width: parent.width
                visible: backend.progress > 0 && backend.progress <= 100
                value: backend.progress / 100.0
            }
        }

        Row {
            id: answers
            width: parent.width
            visible: !page.decided

            BackgroundItem {
                width: parent.width / 2
                height: Theme.itemSizeMedium
                onClicked: page.sayNo()

                Label {
                    anchors.centerIn: parent
                    color: parent.highlighted ? Theme.highlightColor : Theme.primaryColor
                    text: "No"
                }
            }
            BackgroundItem {
                width: parent.width / 2
                height: Theme.itemSizeMedium
                enabled: !page.checking && page.previewError === "" && page.changeText !== "" && !backend.busy
                onClicked: page.sayYes()

                Label {
                    anchors.centerIn: parent
                    color: !parent.enabled ? Theme.secondaryColor
                           : parent.highlighted ? Theme.highlightColor
                           : Theme.primaryColor
                    text: "Yes"
                }
            }
        }
    }
}
