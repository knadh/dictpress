dictpress supports publishing dictionary websites with site themes or templates. Examples:

- [Alar](https://alar.ink) &mdash; Kannada-English dictionary.
- [Olam](https://olam.in) &mdash; English-Malayalam, Malayalam-Malayalam dictionary.

To setup a dictionary website, use the default theme shipped along with the latest [release](https://github.com/knadh/dictpress/releases) in the `site` directory. When running the dictpress binary, pass the path to the directory to it with the `--site` flag.

```shell
./dictpress --site=./site
```

The site will be served on the port set in the configuration file. eg: `http://localhost:9000`. To customize the site, edit the template files in the `site` directory.
