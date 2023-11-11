# About

Tired of getting slammed by scanners and curious as to what they're throwing at your system? Fishler might help.

You can configure it to log every username/password combination thrown at your system.

If you're feeling adventurous you can expose a vicarious and ephemeral Docker container via. authentication to Fishler over SSH.

A container is spun up for any remote IP that connects and authenticates with success - providing a read-only filesystem that you define. Exposing whatever files you would like to 'share'.

## Quickstart

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