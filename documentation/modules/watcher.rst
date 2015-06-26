Watcher
=======

The Watcher is responsible for watching directories for the creation of new files and subdirectories.  New files are submitted to the scheduling queue, and new subdirectories are added to the list of watched directories.


Configuration
-------------

::

    "watcher":
    {
        "active": true,
        "dir": "/data",
        "dirs":
        [
            "/otherdata1",
            "/otherdata2"
        ]
    },

* ``active`` (boolean): Determines whether the Watcher will be active.
* ``dir`` (string; optional\*) specifies a single directory to be watched for new files and subdirectories.  See the :doc:`Concepts <../concepts>` page for details about this directory affects the directory structure.
* ``dirs`` (array of strings; optional\*) specifies a set of directories to be watched for new files and subdirectories.  See the :doc:`Concepts <../concepts>` page for details about these directories affect the directory structure.

\* The watcher must watch at least one directory if it's active, whether specified in ``dir`` or ``dirs``.
