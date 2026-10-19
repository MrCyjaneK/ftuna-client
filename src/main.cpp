#include <sailfishapp.h>

#include <QGuiApplication>
#include <QQuickView>
#include <QScopedPointer>
#include <QtQml>

#include "ftuna.h"

int main(int argc, char *argv[]) {
    QScopedPointer<QGuiApplication> app(SailfishApp::application(argc, argv));
    qmlRegisterType<Ftuna>("eu.ftuna", 1, 0, "Ftuna");
    QScopedPointer<QQuickView> view(SailfishApp::createView());
    view->setSource(SailfishApp::pathToMainQml());
    view->showFullScreen();
    return app->exec();
}
