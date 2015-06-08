Classifier
==========

::

    "classifier":
    {
        "types":
        [
            {
                "name": "egg",
                "match-extension": "egg",
                "do-hash": true,
                "do-nearline": true,
                "nearline-cmd": "echo \"here's an egg file\""
            },
            {
                "name": "rsa-mat",
                "match-extension": "mat",
                "do-hash": true,
                "do-nearline": true,
                "nearline-cmd": "echo \"here's an RSA MAT file\""
            },
            {
                "name": "rsa-setup",
                "match-extension": "Setup",
                "do-hash": false,
                "do-nearline": false
            },
            {
                "name": "regexp-example",
                "match-regexp": "project8is[A-Za-z]*",
                "do-hash": false,
                "do-nearline": false
            }
        ]
    }