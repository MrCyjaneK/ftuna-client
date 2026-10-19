PREFIX ?= /usr
BINDIR ?= $(PREFIX)/bin
LIBDIR ?= $(PREFIX)/lib
DATADIR ?= $(PREFIX)/share
DESTDIR ?=

GO ?= go
export CGO_ENABLED := 1
export GOWORK := off

.PHONY: all install clean

all:
	pkg-config --exists packagekitqt5 || zypper --non-interactive in PackageKit-Qt5-devel
	$(GO) build -mod=vendor -buildmode=c-shared -o libftuna.so ./capi
	moc $$(pkg-config --cflags Qt5Core) src/ftuna.h -o src/moc_ftuna.cpp
	g++ -std=c++11 -fPIC -O2 -Wall \
		$$(pkg-config --cflags sailfishapp Qt5Core Qt5Qml Qt5Quick Qt5Concurrent packagekitqt5) \
		-I. -o ftuna src/main.cpp src/ftuna.cpp src/moc_ftuna.cpp \
		-L. -lftuna \
		$$(pkg-config --libs sailfishapp Qt5Core Qt5Qml Qt5Quick Qt5Concurrent packagekitqt5) \
		-Wl,-rpath,/usr/lib/ftuna -ldl -lpthread

install: all
	install -D -m 755 ftuna $(DESTDIR)$(BINDIR)/ftuna
	install -D -m 755 libftuna.so $(DESTDIR)$(LIBDIR)/ftuna/libftuna.so
	install -D -m 644 ftuna.desktop $(DESTDIR)$(DATADIR)/applications/ftuna.desktop
	install -D -m 644 mapplauncherd/privileges.d/ftuna $(DESTDIR)$(DATADIR)/mapplauncherd/privileges.d/ftuna
	install -D -m 644 qml/ftuna.qml $(DESTDIR)$(DATADIR)/ftuna/qml/ftuna.qml
	install -D -m 644 qml/ui.js $(DESTDIR)$(DATADIR)/ftuna/qml/ui.js
	mkdir -p $(DESTDIR)$(DATADIR)/ftuna/qml/pages $(DESTDIR)$(DATADIR)/ftuna/qml/components $(DESTDIR)$(DATADIR)/ftuna/qml/images
	cp qml/pages/*.qml $(DESTDIR)$(DATADIR)/ftuna/qml/pages/
	cp qml/components/*.qml $(DESTDIR)$(DATADIR)/ftuna/qml/components/
	cp qml/images/*.png qml/images/LICENSE $(DESTDIR)$(DATADIR)/ftuna/qml/images/
	for size in 86 108 128 172 256; do \
		install -D -m 644 icons/$${size}x$${size}/ftuna.png $(DESTDIR)$(DATADIR)/icons/hicolor/$${size}x$${size}/apps/ftuna.png; \
	done

clean:
	rm -f ftuna libftuna.so libftuna.h src/moc_ftuna.cpp
