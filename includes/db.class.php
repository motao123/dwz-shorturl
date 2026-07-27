<?php
// MySQLi database wrapper used by the application.
if (!defined('IN_CRONLITE')) exit();

class DB {
    public $link = null;
    public $result = null;
    public $connect_error = '';

    public function __construct($db_host, $db_user, $db_pass, $db_name, $db_port = 3306) {
        if (!extension_loaded('mysqli')) {
            $this->connect_error = 'MySQLi extension is not installed';
            return;
        }

        // PHP 8 may turn MySQLi failures into exceptions depending on global settings.
        mysqli_report(MYSQLI_REPORT_OFF);
        try {
            $this->link = mysqli_init();
            if (!$this->link) {
                $this->connect_error = 'Unable to initialize MySQLi';
                return;
            }
            mysqli_options($this->link, MYSQLI_OPT_CONNECT_TIMEOUT, 5);
            if (!@mysqli_real_connect($this->link, $db_host, $db_user, $db_pass, $db_name, (int)$db_port)) {
                $this->connect_error = 'Connect Error (' . mysqli_connect_errno() . ') ' . mysqli_connect_error();
                $this->link = null;
                return;
            }
            if (!mysqli_set_charset($this->link, 'utf8mb4')) {
                $this->connect_error = 'Unable to configure database charset';
                mysqli_close($this->link);
                $this->link = null;
            }
        } catch (Throwable $e) {
            $this->connect_error = $e->getMessage();
            $this->link = null;
        }
    }

    public function fetch($query) {
        return $query ? mysqli_fetch_assoc($query) : false;
    }

    public function get_row($query) {
        $result = $this->query($query);
        return $result instanceof mysqli_result ? mysqli_fetch_assoc($result) : false;
    }

    public function count($query) {
        $result = $this->query($query);
        if (!($result instanceof mysqli_result)) return 0;
        $row = mysqli_fetch_row($result);
        return $row ? $row[0] : 0;
    }

    public function query($query) {
        if (!$this->link) return false;
        try {
            $this->result = mysqli_query($this->link, $query);
        } catch (Throwable $e) {
            $this->result = false;
        }
        return $this->result;
    }

    public function prepare($query) {
        if (!$this->link) return false;
        try {
            return mysqli_prepare($this->link, $query);
        } catch (Throwable $e) {
            return false;
        }
    }

    public function escape($value) {
        return $this->link ? mysqli_real_escape_string($this->link, (string)$value) : '';
    }

    public function insert($query) {
        return $this->query($query) ? mysqli_insert_id($this->link) : false;
    }

    public function affected() {
        return $this->link ? mysqli_affected_rows($this->link) : 0;
    }

    public function error() {
        if (!$this->link) return $this->connect_error;
        return '[' . mysqli_errno($this->link) . '] ' . mysqli_error($this->link);
    }

    public function close() {
        if (!$this->link) return true;
        $closed = mysqli_close($this->link);
        $this->link = null;
        return $closed;
    }
}
