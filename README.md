# Fishler

![Code Health](https://github.com/archimoebius/fishler/actions/workflows/golang.yml/badge.svg) | ![Release Status](https://github.com/archimoebius/fishler/actions/workflows/goreleaser.yml/badge.svg)


<p>
    <img src="https://raw.githubusercontent.com/ArchiMoebius/fishler/main/mkdocs/docs/images/logo.png" width="250px" height="250px" alt="logo.png"></br>
    <em style="font-size:0.7em"><a href="https://github.com/invoke-ai/InvokeAI" alt="https://github.com/invoke-ai/InvokeAI" target="_blank">Invoke-AI Generated Logo</a></em>
</p>

A light-weight and easy to deploy SSH honey-pot which leverages Golang and Docker to expose ephemeral shell sessions.

> 💡 Check the [`documentation`](https://archimoebius.github.io/fishler/) for usage and more information.

## Quickstart

### Setup Golang (version of at least 1.21 required)

Download at least version 1.21 of [`Golang`](https://go.dev/dl/) for example:

```bash
sudo su -
wget https://go.dev/dl/go1.21.4.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.21.4.linux-amd64.tar.gz
```

### Setup Fishler

Get up and running quick like:

```
go run github.com/archimoebius/fishler@latest serve --any-account --log-basepath /tmp --crypto-basepath /tmp
```

## Install / Setup

To download fishler and then use it - something like the following will do:

```
go install github.com/archimoebius/fishler@latest
fishler serve --any-account --log-basepath /tmp --crypto-basepath /tmp
```

## Demo

<table stlye="border:0; width: 100%;">
  <tr>
    <td><img src="https://raw.githubusercontent.com/ArchiMoebius/fishler/main/mkdocs/docs/images/server.svg" alt="server.svg"></td>
    <td><img src="https://raw.githubusercontent.com/ArchiMoebius/fishler/main/mkdocs/docs/images/client.svg" alt="client.svg"></td>
  </tr>
</table>

## Credits

Without the shoulders of giants to stand upon - this project wouldn't exist... Thank you for crafting such great libraries!

* [Docker](https://github.com/docker/docker)
* [Gliderlabs/SSH](https://github.com/gliderlabs/ssh)
* [Litter](https://github.com/sanity-io/litter)
* [Logrus](https://github.com/sirupsen/logrus)
* [Cobra](https://github.com/spf13/cobra)
* [Viper](https://github.com/spf13/viper)

## 🤝 Contributing

Contributions, issues, and feature requests are welcome. Feel free to check issues page if you want to contribute.

## 📝 License

Copyright ©2023 ArchiMoebius.

This project is [GPL](https://raw.githubusercontent.com/ArchiMoebius/fishler/main/LICENSE) licensed.