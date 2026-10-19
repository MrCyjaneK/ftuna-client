import QtQuick 2.6
import Sailfish.Silica 1.0

Page {
    id: page
    property var backend
    property var notes: []

    function remember() {
        if (!backend)
            return
        var payload = []
        for (var i = 0; i < notes.length; i++) {
            payload.push({
                title: notes[i].title,
                device_release: notes[i].device_release,
                chosen_release: notes[i].chosen_release,
                arch: notes[i].arch
            })
        }
        backend.rememberFallbacks(JSON.stringify(payload))
    }

    Component.onCompleted: remember()

    SilicaFlickable {
        anchors.fill: parent
        contentHeight: column.height

        Column {
            id: column
            width: parent.width
            spacing: Theme.paddingLarge

            PageHeader {
                title: "Older release"
            }
            Repeater {
                model: notes
                delegate: Label {
                    x: Theme.horizontalPageMargin
                    width: column.width - 2 * x
                    wrapMode: Text.Wrap
                    text: modelData.title + " has no repository for this phone's " + modelData.device_release + " " + modelData.arch + ". Packages from that catalog are " + modelData.chosen_release + " at " + modelData.url
                }
            }
            Button {
                x: Math.round((parent.width - width) / 2)
                text: "Show packages"
                onClicked: pageStack.pop()
            }
        }
    }
}
