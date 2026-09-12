# Avisos de terceiros

## Protocolo da tela

O protocolo da revisão A (`internal/lcd/reva.go`) foi portado para Go a partir do
**turing-smart-screen-python** — Copyright (C) 2021 Matthieu Houdebine (mathoudebine),
https://github.com/mathoudebine/turing-smart-screen-python — licença GPL-3.0-or-later.
Por ser um trabalho derivado, o Bifrost inteiro é distribuído sob a GPL-3.0-or-later
(ver `LICENSE`). O teste `internal/lcd/reva_test.go` confere, byte a byte, que a versão
em Go envia os mesmos comandos que a biblioteca original.

## Fontes

**Roboto** e **Roboto Mono** (Google), licença Apache 2.0 — `internal/render/fonts/LICENSE-Roboto.txt`.

## Bibliotecas Go

| Biblioteca | Uso | Licença |
|---|---|---|
| github.com/saltosystems/winrt-go | Controle de Mídia do Windows (WinRT) | MIT |
| github.com/go-ole/go-ole | COM/WinRT | MIT |
| github.com/fogleman/gg | desenho das telas | MIT |
| github.com/golang/freetype | renderização de fontes | FreeType License (à escolha: FTL ou GPL-2.0+) |
| golang.org/x/image, golang.org/x/sys | imagens e APIs do Windows | BSD-3-Clause |
| go.bug.st/serial | listar portas COM | BSD-3-Clause |
| fyne.io/systray | ícone perto do relógio | Apache 2.0 |
| github.com/jchv/go-webview2 | janela do painel (WebView2) | MIT |
| github.com/tc-hib/winres | ícone e manifesto do .exe | 0BSD |

## Serviços consultados

- **Steam Web API** (Valve) — jogo atual e tempo jogado; capas vindas do CDN público da Steam.
