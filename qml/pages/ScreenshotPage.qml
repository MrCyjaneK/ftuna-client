import QtQuick 2.6
import Sailfish.Silica 1.0
import Sailfish.Gallery 1.0
import "../ui.js" as Ui

Page {
    property var paths: []
    property int index: 0

    SlideshowView {
        id: slides
        anchors.fill: parent
        model: paths
        currentIndex: index
        itemWidth: width
        delegate: ImageViewer {
            width: slides.itemWidth
            height: slides.height
            source: Ui.picture(modelData)
            active: PathView.isCurrentItem
        }
    }
}
