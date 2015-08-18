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
        ],
        "file-wait-time": "5s"
    },

* ``active`` (boolean): Determines whether the Watcher will be active.
* ``dir`` (string; optional\*) specifies a single directory to be watched for new files and subdirectories.  See the :doc:`Concepts <../concepts>` page for details about this directory affects the directory structure.
* ``dirs`` (array of strings; optional\*) specifies a set of directories to be watched for new files and subdirectories.  See the :doc:`Concepts <../concepts>` page for details about these directories affect the directory structure.
* ``file-wait-time`` (string; optional (default = 5s)) specifies the amount of time that hornet will wait between finding the file and submitting it for further processing; the format for the parameter is an integer or decimal number followed by a unit suffix: e.g. 2s, 1.5s, 1s500ms.  Valid time units are ``ns``, ``us``, ``ms``, ``s``, ``m``, and ``h``.

\* The watcher must watch at least one directory if it's active, whether specified in ``dir`` or ``dirs``.
