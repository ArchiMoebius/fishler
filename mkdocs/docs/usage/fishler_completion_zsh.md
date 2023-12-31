## fishler completion zsh

Generate the autocompletion script for zsh

### Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(fishler completion zsh)

To load completions for every new session, execute once:

#### Linux:

	fishler completion zsh > "${fpath[1]}/_fishler"

#### macOS:

	fishler completion zsh > $(brew --prefix)/share/zsh/site-functions/_fishler

You will need to start a new shell for this setup to take effect.


```
fishler completion zsh [flags]
```

### Options

```
  -h, --help              help for zsh
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
  -c, --config string             .fishler.yaml (default ".fishler")
  -d, --debug                     Output debug information
      --docker-basepath string    The path to the docker folder ./docker if run from the root of the project (default "docker")
      --docker-imagename string   The image user for the docker container (default "fishler")
  -l, --log-basepath string       The base filepath where logs will be stored (default "/var/log/fishler")
```

### SEE ALSO

* [fishler completion](fishler_completion.md)	 - Generate the autocompletion script for the specified shell

###### Auto generated by spf13/cobra on 24-Dec-2023
