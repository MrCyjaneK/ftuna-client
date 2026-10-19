import QtQuick 2.6
import Sailfish.Silica 1.0
import "../ui.js" as Ui

Dialog {
    id: dialog

    property var listPage
    property int editIndex: -1
    property bool builtin: false
    property string editName: ""
    property string editURL: ""
    property bool editEnabled: true
    property string editDetail: ""
    property string editStatus: ""
    property string editId: ""

    canAccept: /^https:\/\/[^\/\s]+/.test(Ui.trim(urlField.text))

    onAccepted: {
        if (!listPage)
            return
        listPage.applyRepo(editIndex, {
            title: nameField.text,
            url: urlField.text,
            enabled: enabledSwitch.checked
        })
    }

    SilicaFlickable {
        anchors.fill: parent
        contentHeight: column.height

        Column {
            id: column
            width: parent.width

            DialogHeader {
                title: editIndex < 0 ? "Repository" : (editName || "Repository")
                acceptText: editIndex < 0 ? "Add" : "Save"
            }

            TextSwitch {
                id: enabledSwitch
                width: parent.width
                text: "Enabled"
                description: editDetail
                checked: editEnabled
            }

            TextField {
                id: nameField
                width: parent.width
                text: editName
                label: "Name"
                placeholderText: "My repository"
                EnterKey.iconSource: "image://theme/icon-m-enter-next"
                EnterKey.enabled: true
                EnterKey.onClicked: urlField.focus = true
            }

            TextField {
                id: urlField
                width: parent.width
                text: editURL
                label: "Repository URL"
                placeholderText: "https://example.com/{version}/{arch}/"
                inputMethodHints: Qt.ImhNoAutoUppercase | Qt.ImhNoPredictiveText
                EnterKey.enabled: dialog.canAccept
                EnterKey.iconSource: "image://theme/icon-m-enter-accept"
                EnterKey.onClicked: dialog.accept()
            }

            Label {
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                visible: editStatus !== ""
                wrapMode: Text.Wrap
                color: Theme.highlightColor
                text: editStatus
            }

            BackgroundItem {
                width: parent.width
                height: visible ? Theme.itemSizeSmall : 0
                visible: editIndex >= 0
                onClicked: pageStack.push(Qt.resolvedUrl("RepoPackagesPage.qml"), {
                    backend: listPage ? listPage.backend : null,
                    prefId: editId,
                    title: nameField.text || editName || "Packages",
                    repoOn: enabledSwitch.checked
                })

                Label {
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.horizontalPageMargin
                    anchors.right: packageChevron.left
                    anchors.rightMargin: Theme.paddingMedium
                    anchors.verticalCenter: parent.verticalCenter
                    truncationMode: TruncationMode.Fade
                    color: parent.highlighted ? Theme.highlightColor : Theme.primaryColor
                    text: "Packages"
                }
                Image {
                    id: packageChevron
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.horizontalPageMargin
                    anchors.verticalCenter: parent.verticalCenter
                    source: "image://theme/icon-m-right?" + (parent.highlighted ? Theme.highlightColor : Theme.primaryColor)
                }
            }

            BackgroundItem {
                width: parent.width
                height: visible ? Theme.itemSizeSmall : 0
                visible: editIndex >= 0 && !builtin
                onClicked: remorse.execute("Removing", function() {
                    if (listPage)
                        listPage.removeRepo(editIndex)
                    dialog.reject()
                })

                Label {
                    anchors.centerIn: parent
                    color: parent.highlighted ? Theme.highlightColor : "#e15b64"
                    text: "Remove"
                }
            }
        }

        VerticalScrollDecorator {}
    }

    RemorsePopup { id: remorse }
}
