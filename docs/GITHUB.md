# Publishing to github.com/asechoud

This project is intended to live at:

**https://github.com/asechoud/bmp-bgp-msg-stats**

## Create the public repository

From an account that can create repos under [asechoud](https://github.com/asechoud):

```bash
gh repo create asechoud/bmp-bgp-msg-stats \
  --public \
  --description "BMP BGP session PDU counters (draft-choudhary-grow-bmp-bgp-msg-stats-00)" \
  --source . \
  --remote github \
  --push
```

Or create an empty public repo in the GitHub UI, then:

```bash
git remote add github https://github.com/asechoud/bmp-bgp-msg-stats.git
git push -u github main
```

## Cursor / cloud agent note

The Cursor session that authored this code was authenticated to Origin and to
GitHub account `vaishalic-git`, which cannot create or push repositories under
`asechoud`. Connect the `asechoud` GitHub account (or a token with
`repo` scope for that user) to finish the publish step above.
