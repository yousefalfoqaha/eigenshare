# Contributing

Any contribution to Eigenshare would be greatly appreciated, including issues, suggestions, pull requests and more.

## Getting started

If you have found a bug, have a suggestion, or something else, just create an issue on GitHub and we can get in touch
🙌.

## Submit a pull request

Before you submit the pull request for review please ensure that

- The pull request naming follows the [Conventional Commits specification](https://www.conventionalcommits.org):

  `<type>[optional scope]: <description>`

  example:

  ```
  feat(api): add email sharing
  ```

  When `TYPE` can be:

  - **feat** - is a new feature
  - **docs** - documentation only changes
  - **fix** - a bug fix
  - **refactor** - code change that neither fixes a bug nor adds a feature

- Your pull request has a detailed description

## Development setup

1. Make sure to install Golang `1.26.1`.
2. Clone the repository.
3. Set the `SECURE_COOKIES` environment variable to `false`.
4. Run `go run .` at the cloned location.

The dev server will be listening on `http://localhost:8080`.
