Mover
=====

The Mover is responsible for transferring each file from the hot storage location to the warm storage location.  It performs the following sequence of actions on each file:

1. Copy the file from its original location to the destination directory.
2. If the file hash is provided, perform a hash of the copied file and check whether it matches.  Failure to match the hash is currently a fatal error.
3. Remove the file from the original location.

Configuration
-------------

::

    "mover":
    {
        "dest-dir": "/warm-data"
    },

* ``dest-dir`` (string): destination directory to which files are moved.  See the :doc:`Concepts <../concepts>` page for details about the directory structure.  This must be a valid path or Hornet will exit.

