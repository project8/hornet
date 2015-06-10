Hashing
=======

The hashing configuration specifies how files are hashed.  File hashes are typically used for two purposes:

1. To ensure that files copied from one location to another (in particular, from the hot storage to the warm storage) are unchanged (i.e. no random bit flips);

2. To serve as a means of file identification in the run database that doesn't depend on the filename.

Configuration
-------------

::

	"hash":
	{
		"required": true,
		"command": "md5sum",
		"cmd-opt": "-b"
	}

* ``required`` (boolean): determines whether a hash failure is a fatal error for hornet.  If true, and a file type requests a hash, if the hash fails, then hornet will report the error and exit.
* ``command`` (string): the command used to run the hash.
* ``cmd-opt`` (string): the options passed to the command before the filename.

The hash command used is built from the configuration and the input filename::

	> [command] [cmd-opt] [filename]
