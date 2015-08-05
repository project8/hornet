Shipper
=======

The Shipper is responsible for moving files from the warm storage location to the cold storage location.  This is done using the ``rsync`` program.  The cold storage location may be locally mounted or remotely accessed via a network connection.

If remote cold storage is used, it is strongly suggested that you use an RSA key to log into the remote system, so that a password does not need to be entered for every file transfer.

Configuration
-------------

::

    "shipper":
    {
        "n-shippers": 1,
        "dest-dir": "/remote-data",
        "hostname": "my.server",
        "username": "aphysicist"
    }

* ``n-shippers`` (unsigned int): currently this must be 1.  In a future release it will be possible to have more than one shipper to increase the data transfer rate. **(dev)** This value is processed by the Scheduler.
* ``dest-dir`` (string): destination directory to which the files are shipped.  See the :doc:`Concepts <../concepts>` page for details about the directory structure.  If this is not a valid path, the rsync transfers will fail.
* ``hostname`` (string; optional): if this is present and is not an empty string, then the ``hostname`` will prefix the ``dest-dir`` in the ``rsync`` command: ``[hostname]:[dest-dir]``.
* ``username`` (string; optional): if this is present and is not an empty string, and if there is a ``hostname`` given, then the ``username`` will prefix the hostname in the ``rsync`` command: ``[username]@[hostname]:[dest-dir]``.
